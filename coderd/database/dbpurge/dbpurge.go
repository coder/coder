package dbpurge

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/objstore"
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
	// Batch sizes for chat purging. Both use 1000, which is smaller
	// than audit/connection log batches (10000), because chat_files
	// rows contain bytea blob data that make large batches heavier.
	chatsBatchSize     = 1000
	chatFilesBatchSize = 1000
)

// chatFilesNamespace is the object store namespace under which chat
// files are stored.
const chatFilesNamespace = "chatfiles"

// New creates a new periodically purging database instance.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for cleaning up old, unused resources from the database that take up space.
func New(ctx context.Context, logger slog.Logger, db database.Store, vals *codersdk.DeploymentValues, clk quartz.Clock, reg prometheus.Registerer, objStore objstore.Store) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // Use dbpurge-specific subject with minimal permissions.
	ctx = dbauthz.AsDBPurge(ctx)

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

	objStoreInflight := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "dbpurge",
		Name:      "objstore_delete_inflight",
		Help:      "Number of object store files currently enqueued for deletion.",
	})
	reg.MustRegister(objStoreInflight)

	objStoreDeleted := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "dbpurge",
		Name:      "objstore_files_deleted_total",
		Help:      "Total number of object store files successfully deleted.",
	})
	reg.MustRegister(objStoreDeleted)

	inst := &instance{
		cancel:            cancelFunc,
		closed:            closed,
		logger:            logger,
		vals:              vals,
		clk:               clk,
		iterationDuration: iterationDuration,
		recordsPurged:     recordsPurged,
		objStore:          objStore,
		objStoreInflight:  objStoreInflight,
		objStoreDeleted:   objStoreDeleted,
	}

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(ctx context.Context, start time.Time) {
		defer ticker.Reset(delay)
		err := inst.purgeTick(ctx, db, start)
		if err != nil {
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))

			// Record metrics for failed purge iteration.
			duration := clk.Since(start)
			iterationDuration.WithLabelValues("false").Observe(duration.Seconds())
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
	return inst
}

// purgeTick performs a single purge iteration. It returns an error if the
// purge fails.
func (i *instance) purgeTick(ctx context.Context, db database.Store, start time.Time) error {
	// Read chat retention config outside the transaction to
	// avoid poisoning the tx if the stored value is corrupt.
	// A SQL-level cast error (e.g. non-numeric text) puts PG
	// into error state, failing all subsequent queries in the
	// same transaction.
	chatRetentionDays, err := db.GetChatRetentionDays(ctx)
	if err != nil {
		i.logger.Warn(ctx, "failed to read chat retention config, skipping chat purge", slog.Error(err))
		chatRetentionDays = 0
	}

	// Start a transaction to grab advisory lock, we don't want to run
	// multiple purges at the same time (multiple replicas).
	return db.InTx(func(tx database.Store) error {
		// Acquire a lock to ensure that only one instance of the
		// purge is running at a time.
		ok, err := tx.TryAcquireLock(ctx, database.LockIDDBPurge)
		if err != nil {
			return err
		}
		if !ok {
			i.logger.Debug(ctx, "unable to acquire lock for purging old database entries, skipping")
			return nil
		}

		var purgedWorkspaceAgentLogs int64
		workspaceAgentLogsRetention := i.vals.Retention.WorkspaceAgentLogs.Value()
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
		apiKeysRetention := i.vals.Retention.APIKeys.Value()
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
		aibridgeRetention := i.vals.AI.BridgeConfig.Retention.Value()
		if aibridgeRetention > 0 {
			deleteAIBridgeRecordsBefore := start.Add(-aibridgeRetention)
			// nolint:gocritic // Needs to run as aibridge context.
			purgedAIBridgeRecords, err = tx.DeleteOldAIBridgeRecords(dbauthz.AsAIBridged(ctx), deleteAIBridgeRecordsBefore)
			if err != nil {
				return xerrors.Errorf("failed to delete old aibridge records: %w", err)
			}
		}

		var purgedConnectionLogs int64
		connectionLogsRetention := i.vals.Retention.ConnectionLogs.Value()
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
		auditLogsRetention := i.vals.Retention.AuditLogs.Value()
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

		// Chat retention is configured via site_configs. When
		// enabled, old archived chats are deleted first, then
		// orphaned chat files. Deleting a chat cascades to
		// chat_file_links (removing references) but not to
		// chat_files directly, so files from deleted chats
		// become orphaned and are caught by DeleteOldChatFiles
		// in the same tick.
		var purgedChats int64
		var purgedChatFiles int64
		if chatRetentionDays > 0 {
			chatRetention := time.Duration(chatRetentionDays) * 24 * time.Hour
			deleteChatsBefore := start.Add(-chatRetention)

			purgedChats, err = tx.DeleteOldChats(ctx, database.DeleteOldChatsParams{
				BeforeTime: deleteChatsBefore,
				LimitCount: chatsBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old chats: %w", err)
			}

			deletedFiles, err := tx.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
				BeforeTime: deleteChatsBefore,
				LimitCount: chatFilesBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old chat files: %w", err)
			}
			purgedChatFiles = int64(len(deletedFiles))

			// Collect object store keys from the deleted rows
			// and delete them in a background goroutine so
			// slow object store I/O does not hold the
			// advisory lock or block the next tick.
			i.deleteObjStoreKeys(ctx, deletedFiles)
		}
		i.logger.Debug(ctx, "purged old database entries",
			slog.F("workspace_agent_logs", purgedWorkspaceAgentLogs),
			slog.F("expired_api_keys", expiredAPIKeys),
			slog.F("aibridge_records", purgedAIBridgeRecords),
			slog.F("connection_logs", purgedConnectionLogs),
			slog.F("audit_logs", purgedAuditLogs),
			slog.F("chats", purgedChats),
			slog.F("chat_files", purgedChatFiles),
			slog.F("duration", i.clk.Since(start)),
		)

		if i.iterationDuration != nil {
			duration := i.clk.Since(start)
			i.iterationDuration.WithLabelValues("true").Observe(duration.Seconds())
		}
		if i.recordsPurged != nil {
			i.recordsPurged.WithLabelValues("workspace_agent_logs").Add(float64(purgedWorkspaceAgentLogs))
			i.recordsPurged.WithLabelValues("expired_api_keys").Add(float64(expiredAPIKeys))
			i.recordsPurged.WithLabelValues("aibridge_records").Add(float64(purgedAIBridgeRecords))
			i.recordsPurged.WithLabelValues("connection_logs").Add(float64(purgedConnectionLogs))
			i.recordsPurged.WithLabelValues("audit_logs").Add(float64(purgedAuditLogs))
			i.recordsPurged.WithLabelValues("chats").Add(float64(purgedChats))
			i.recordsPurged.WithLabelValues("chat_files").Add(float64(purgedChatFiles))
		}

		return nil
	}, database.DefaultTXOptions().WithID("db_purge"))
}

type instance struct {
	cancel            context.CancelFunc
	closed            chan struct{}
	logger            slog.Logger
	vals              *codersdk.DeploymentValues
	clk               quartz.Clock
	iterationDuration *prometheus.HistogramVec
	recordsPurged     *prometheus.CounterVec
	objStore          objstore.Store
	objStoreInflight  prometheus.Gauge
	objStoreDeleted   prometheus.Counter

	// objDeleteMu serializes background object store delete batches
	// so at most one goroutine is deleting at a time.
	objDeleteMu sync.Mutex
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}

// deleteObjStoreKeys removes object store entries for the given
// deleted chat file rows. The work runs in a background goroutine
// guarded by a mutex so that slow object store I/O never blocks
// the purge transaction or the next tick. At most one delete batch
// runs at a time; if a batch is already in flight the new keys are
// silently dropped (they will be orphan-collected on a future tick
// if needed).
func (i *instance) deleteObjStoreKeys(ctx context.Context, rows []database.DeleteOldChatFilesRow) {
	// Collect non-empty object store keys.
	var keys []string
	for _, r := range rows {
		if r.ObjectStoreKey.Valid && r.ObjectStoreKey.String != "" {
			keys = append(keys, r.ObjectStoreKey.String)
		}
	}
	if len(keys) == 0 {
		return
	}

	// Try to acquire the mutex without blocking. If another
	// delete batch is already running, skip this one.
	if !i.objDeleteMu.TryLock() {
		i.logger.Debug(ctx, "object store delete already in progress, skipping batch",
			slog.F("skipped_keys", len(keys)))
		return
	}

	i.objStoreInflight.Add(float64(len(keys)))

	go func() {
		defer i.objDeleteMu.Unlock()

		var deleted int
		for _, key := range keys {
			if ctx.Err() != nil {
				remaining := len(keys) - deleted
				i.objStoreInflight.Sub(float64(remaining))
				i.logger.Debug(ctx, "context canceled during object store cleanup",
					slog.F("deleted", deleted),
					slog.F("remaining", remaining))
				return
			}
			if err := i.objStore.Delete(ctx, chatFilesNamespace, key); err != nil {
				i.logger.Warn(ctx, "failed to delete chat file from object store",
					slog.F("key", key),
					slog.Error(err))
			} else {
				deleted++
			}
			i.objStoreInflight.Dec()
		}

		i.objStoreDeleted.Add(float64(deleted))
		i.logger.Debug(ctx, "deleted chat files from object store",
			slog.F("deleted", deleted),
			slog.F("failed", len(keys)-deleted))
	}()
}
