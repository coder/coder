package dbpurge

import (
	"context"
	"io"
	"time"

	"golang.org/x/xerrors"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	delay = 10 * time.Minute
	// Connection events are now inserted into the `connection_logs` table.
	// We'll slowly remove old connection events from the `audit_logs` table.
	// The `connection_logs` table is purged based on the configured retention.
	maxAuditLogConnectionEventAge    = 90 * 24 * time.Hour // 90 days
	auditLogConnectionEventBatchSize = 1000
	// Batch size for connection log deletion.
	connectionLogsBatchSize = 10000
	// Batch size for audit log deletion.
	auditLogsBatchSize = 10000
	// Telemetry heartbeats are used to deduplicate events across replicas. We
	// don't need to persist heartbeat rows for longer than 24 hours, as they
	// are only used for deduplication across replicas. The time needs to be
	// long enough to cover the maximum interval of a heartbeat event (currently
	// 1 hour) plus some buffer.
	maxTelemetryHeartbeatAge = 24 * time.Hour
)

// New creates a new periodically purging database instance.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for cleaning up old, unused resources from the database that take up space.
func New(ctx context.Context, logger slog.Logger, db database.Store, vals *codersdk.DeploymentValues, clk quartz.Clock, reg prometheus.Registerer) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system purges old db records without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	iterationDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "dbpurge",
		Name:      "iteration_duration_seconds",
		Help:      "Duration of each dbpurge iteration in seconds.",
		Buckets:   []float64{1, 5, 10, 30, 60, 300, 600}, // 1s to 10min
	}, []string{"success"})
	reg.MustRegister(iterationDuration)

	recordsPurged := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "dbpurge",
		Name:      "records_purged_total",
		Help:      "Total number of records purged by type.",
	}, []string{"record_type"})
	reg.MustRegister(recordsPurged)

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(ctx context.Context, start time.Time) {
		defer ticker.Reset(delay)
		// Start a transaction to grab advisory lock, we don't want to run
		// multiple purges at the same time (multiple replicas).
		if err := db.InTx(func(tx database.Store) error {
			// Acquire a lock to ensure that only one instance of the
			// purge is running at a time.
			ok, err := tx.TryAcquireLock(ctx, database.LockIDDBPurge)
			if err != nil {
				return err
			}
			if !ok {
				logger.Debug(ctx, "unable to acquire lock for purging old database entries, skipping")
				return nil
			}

			var purgedWorkspaceAgentLogs int64
			workspaceAgentLogsRetention := vals.Retention.WorkspaceAgentLogs.Value()
			if workspaceAgentLogsRetention > 0 {
				deleteOldWorkspaceAgentLogsBefore := start.Add(-workspaceAgentLogsRetention)
				purgedWorkspaceAgentLogs, err = tx.DeleteOldWorkspaceAgentLogs(ctx, deleteOldWorkspaceAgentLogsBefore)
				if err != nil {
					return xerrors.Errorf("failed to delete old workspace agent logs: %w", err)
				}
			}
			if err := tx.DeleteOldWorkspaceAgentStats(ctx); err != nil {
				return xerrors.Errorf("failed to delete old workspace agent stats: %w", err)
			}
			if err := tx.DeleteOldProvisionerDaemons(ctx); err != nil {
				return xerrors.Errorf("failed to delete old provisioner daemons: %w", err)
			}
			if err := tx.DeleteOldNotificationMessages(ctx); err != nil {
				return xerrors.Errorf("failed to delete old notification messages: %w", err)
			}
			if err := tx.ExpirePrebuildsAPIKeys(ctx, dbtime.Time(start)); err != nil {
				return xerrors.Errorf("failed to expire prebuilds user api keys: %w", err)
			}

			var expiredAPIKeys int64
			apiKeysRetention := vals.Retention.APIKeys.Value()
			if apiKeysRetention > 0 {
				// Delete keys that have been expired for at least the retention period.
				// A higher retention period allows the backend to return a more helpful
				// error message when a user tries to use an expired key.
				deleteExpiredKeysBefore := start.Add(-apiKeysRetention)
				expiredAPIKeys, err = tx.DeleteExpiredAPIKeys(ctx, database.DeleteExpiredAPIKeysParams{
					Before: dbtime.Time(deleteExpiredKeysBefore),
					// There could be a lot of expired keys here, so set a limit to prevent
					// this taking too long. This runs every 10 minutes, so it deletes
					// ~1.5m keys per day at most.
					LimitCount: 10000,
				})
				if err != nil {
					return xerrors.Errorf("failed to delete expired api keys: %w", err)
				}
			}
			deleteOldTelemetryLocksBefore := start.Add(-maxTelemetryHeartbeatAge)
			if err := tx.DeleteOldTelemetryLocks(ctx, deleteOldTelemetryLocksBefore); err != nil {
				return xerrors.Errorf("failed to delete old telemetry locks: %w", err)
			}

			deleteOldAuditLogConnectionEventsBefore := start.Add(-maxAuditLogConnectionEventAge)
			if err := tx.DeleteOldAuditLogConnectionEvents(ctx, database.DeleteOldAuditLogConnectionEventsParams{
				BeforeTime: deleteOldAuditLogConnectionEventsBefore,
				LimitCount: auditLogConnectionEventBatchSize,
			}); err != nil {
				return xerrors.Errorf("failed to delete old audit log connection events: %w", err)
			}

			var purgedAIBridgeRecords int64
			aibridgeRetention := vals.AI.BridgeConfig.Retention.Value()
			if aibridgeRetention > 0 {
				deleteAIBridgeRecordsBefore := start.Add(-aibridgeRetention)
				// nolint:gocritic // Needs to run as aibridge context.
				purgedAIBridgeRecords, err = tx.DeleteOldAIBridgeRecords(dbauthz.AsAIBridged(ctx), deleteAIBridgeRecordsBefore)
				if err != nil {
					return xerrors.Errorf("failed to delete old aibridge records: %w", err)
				}
			}

			var purgedConnectionLogs int64
			connectionLogsRetention := vals.Retention.ConnectionLogs.Value()
			if connectionLogsRetention > 0 {
				deleteConnectionLogsBefore := start.Add(-connectionLogsRetention)
				purgedConnectionLogs, err = tx.DeleteOldConnectionLogs(ctx, database.DeleteOldConnectionLogsParams{
					BeforeTime: deleteConnectionLogsBefore,
					LimitCount: connectionLogsBatchSize,
				})
				if err != nil {
					return xerrors.Errorf("failed to delete old connection logs: %w", err)
				}
			}

			var purgedAuditLogs int64
			auditLogsRetention := vals.Retention.AuditLogs.Value()
			if auditLogsRetention > 0 {
				deleteAuditLogsBefore := start.Add(-auditLogsRetention)
				purgedAuditLogs, err = tx.DeleteOldAuditLogs(ctx, database.DeleteOldAuditLogsParams{
					BeforeTime: deleteAuditLogsBefore,
					LimitCount: auditLogsBatchSize,
				})
				if err != nil {
					return xerrors.Errorf("failed to delete old audit logs: %w", err)
				}
			}

			logger.Debug(ctx, "purged old database entries",
				slog.F("workspace_agent_logs", purgedWorkspaceAgentLogs),
				slog.F("expired_api_keys", expiredAPIKeys),
				slog.F("aibridge_records", purgedAIBridgeRecords),
				slog.F("connection_logs", purgedConnectionLogs),
				slog.F("audit_logs", purgedAuditLogs),
				slog.F("duration", clk.Since(start)),
			)

			duration := clk.Since(start)
			iterationDuration.WithLabelValues("true").Observe(duration.Seconds())
			recordsPurged.WithLabelValues("workspace_agent_logs").Add(float64(purgedWorkspaceAgentLogs))
			recordsPurged.WithLabelValues("expired_api_keys").Add(float64(expiredAPIKeys))
			recordsPurged.WithLabelValues("aibridge_records").Add(float64(purgedAIBridgeRecords))
			recordsPurged.WithLabelValues("connection_logs").Add(float64(purgedConnectionLogs))
			recordsPurged.WithLabelValues("audit_logs").Add(float64(purgedAuditLogs))

			return nil
		}, database.DefaultTXOptions().WithID("db_purge")); err != nil {
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))

			// Record metrics for failed purge iteration.
			duration := clk.Since(start)
			iterationDuration.WithLabelValues("false").Observe(duration.Seconds())

			return
		}
	}

	pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceDBPurge), func(ctx context.Context) {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(ctx, dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				ticker.Stop()
				doTick(ctx, dbtime.Time(tick).UTC())
			}
		}
	})
	return &instance{
		cancel: cancelFunc,
		closed: closed,
	}
}

type instance struct {
	cancel context.CancelFunc
	closed chan struct{}
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}
