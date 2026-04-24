package dbpurge

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
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
	// Chat batch sizes stay smaller than audit/connection log batches because
	// chat_files rows carry bytea blobs.
	chatsBatchSize     = 1000
	chatFilesBatchSize = 1000
)

// defaultChatAutoArchiveBatchSize bounds how many root chats one
// tick will archive by default.
const defaultChatAutoArchiveBatchSize int32 = 1000

type Option func(*instance)

// WithClock overrides the clock used by the purger. Defaults to
// quartz.NewReal().
func WithClock(clk quartz.Clock) Option {
	return func(i *instance) { i.clk = clk }
}

// WithChatAutoArchiveBatchSize overrides how many root chats a
// single tick will auto-archive. Defaults to
// defaultChatAutoArchiveBatchSize (1000).
func WithChatAutoArchiveBatchSize(n int32) Option {
	return func(i *instance) { i.chatAutoArchiveBatchSize = n }
}

// New creates a new periodically purging database instance.
// Callers must Close the returned instance.
//
// The auditor pointer is loaded on each dispatch tick so runtime
// entitlement changes (e.g. toggling the audit-log feature) take
// effect without restarting the process.
func New(ctx context.Context, logger slog.Logger, db database.Store, vals *codersdk.DeploymentValues, reg prometheus.Registerer, auditor *atomic.Pointer[audit.Auditor], opts ...Option) io.Closer {
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

	chatAutoArchiveRecords := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "chat_auto_archive",
		Name:      "records_archived_total",
		Help:      "Total number of chats archived by the auto-archive job (counting both roots and cascaded children).",
	})
	reg.MustRegister(chatAutoArchiveRecords)

	inst := &instance{
		cancel:                   cancelFunc,
		closed:                   closed,
		logger:                   logger,
		vals:                     vals,
		clk:                      quartz.NewReal(),
		auditor:                  auditor,
		iterationDuration:        iterationDuration,
		recordsPurged:            recordsPurged,
		chatAutoArchiveRecords:   chatAutoArchiveRecords,
		chatAutoArchiveBatchSize: defaultChatAutoArchiveBatchSize,
	}
	for _, opt := range opts {
		opt(inst)
	}

	// Start the ticker with the initial delay.
	ticker := inst.clk.NewTicker(delay)
	doTick := func(ctx context.Context, start time.Time) {
		defer ticker.Reset(delay)
		err := inst.purgeTick(ctx, db, start)
		if err != nil {
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))

			// Record metrics for failed purge iteration.
			duration := inst.clk.Since(start)
			iterationDuration.WithLabelValues("false").Observe(duration.Seconds())
		}
	}

	pproflabel.Go(ctx, pproflabel.Service(pproflabel.ServiceDBPurge), func(ctx context.Context) {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(ctx, dbtime.Time(inst.clk.Now()).UTC())
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

	// Same rationale as chat_retention_days: read outside the tx.
	chatAutoArchiveDays, err := db.GetChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays)
	if err != nil {
		i.logger.Warn(ctx, "failed to read chat auto-archive config, skipping auto-archive", slog.Error(err))
		chatAutoArchiveDays = 0
	}

	// Populated inside the tx; dispatched post-commit.
	var archivedChats []database.AutoArchiveInactiveChatsRow

	// Start a transaction to grab advisory lock, we don't want to run
	// multiple purges at the same time (multiple replicas).
	err = db.InTx(func(tx database.Store) error {
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

			purgedChatFiles, err = tx.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
				BeforeTime: deleteChatsBefore,
				LimitCount: chatFilesBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old chat files: %w", err)
			}
		}

		// Auto-archive runs after the delete pass so newly
		// archived chats aren't eligible for deletion this tick.
		if chatAutoArchiveDays > 0 {
			archiveCutoff := start.Add(-time.Duration(chatAutoArchiveDays) * 24 * time.Hour)
			archivedChats, err = tx.AutoArchiveInactiveChats(ctx, database.AutoArchiveInactiveChatsParams{
				ArchiveCutoff: archiveCutoff,
				LimitCount:    i.chatAutoArchiveBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to auto-archive inactive chats: %w", err)
			}
		}

		i.logger.Debug(ctx, "purged old database entries",
			slog.F("workspace_agent_logs", purgedWorkspaceAgentLogs),
			slog.F("expired_api_keys", expiredAPIKeys),
			slog.F("aibridge_records", purgedAIBridgeRecords),
			slog.F("connection_logs", purgedConnectionLogs),
			slog.F("audit_logs", purgedAuditLogs),
			slog.F("chats", purgedChats),
			slog.F("chat_files", purgedChatFiles),
			slog.F("auto_archived_chats", len(archivedChats)),
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
	if err != nil {
		return err
	}

	// Dispatch audits post-commit on a detached context so ticker
	// cancellation doesn't interrupt the loop. No timeout: every root
	// must be audited to avoid gaps in the trail. Children inherit
	// their root's archival decision and are not audited individually,
	// matching the manual archive path (patchChat audits the root only).
	if len(archivedChats) > 0 {
		i.chatAutoArchiveRecords.Add(float64(len(archivedChats)))
		dispatchCtx := context.WithoutCancel(ctx)
		i.dispatchChatAutoArchive(dispatchCtx, archivedChats)
	}

	return nil
}

type instance struct {
	cancel                   context.CancelFunc
	closed                   chan struct{}
	logger                   slog.Logger
	vals                     *codersdk.DeploymentValues
	clk                      quartz.Clock
	auditor                  *atomic.Pointer[audit.Auditor]
	iterationDuration        *prometheus.HistogramVec
	recordsPurged            *prometheus.CounterVec
	chatAutoArchiveRecords   prometheus.Counter
	chatAutoArchiveBatchSize int32
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}

// chatFromAutoArchiveRow reshapes the query row into a database.Chat for
// audit.Auditable[database.Chat].
func chatFromAutoArchiveRow(logger slog.Logger, r database.AutoArchiveInactiveChatsRow) database.Chat {
	var labels database.StringMap
	// sqlc's StringMap override doesn't reach CTE-aliased columns, so Labels
	// arrives as raw JSON bytes. StringMap.Scan handles []byte and nil.
	if err := labels.Scan([]byte(r.Labels)); err != nil {
		logger.Warn(context.Background(), "failed to parse chat labels from auto-archive row",
			slog.F("chat_id", r.ID),
			slog.F("raw_labels", string(r.Labels)),
			slog.Error(err),
		)
	}
	return database.Chat{
		ID:                  r.ID,
		OwnerID:             r.OwnerID,
		OrganizationID:      r.OrganizationID,
		WorkspaceID:         r.WorkspaceID,
		BuildID:             r.BuildID,
		AgentID:             r.AgentID,
		Title:               r.Title,
		Status:              r.Status,
		WorkerID:            r.WorkerID,
		StartedAt:           r.StartedAt,
		HeartbeatAt:         r.HeartbeatAt,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
		ParentChatID:        r.ParentChatID,
		RootChatID:          r.RootChatID,
		LastModelConfigID:   r.LastModelConfigID,
		Archived:            r.Archived,
		LastError:           r.LastError,
		Mode:                r.Mode,
		MCPServerIDs:        r.MCPServerIDs,
		Labels:              labels,
		PinOrder:            r.PinOrder,
		LastReadMessageID:   r.LastReadMessageID,
		LastInjectedContext: r.LastInjectedContext,
		DynamicTools:        r.DynamicTools,
		PlanMode:            r.PlanMode,
		ClientType:          r.ClientType,
	}
}

// dispatchChatAutoArchive audits every archived root chat. Children
// inherit their root's archival decision and are skipped, matching
// the manual archive path (patchChat audits the root only). Runs on
// a detached context so ticker cancellation cannot truncate the trail.
func (i *instance) dispatchChatAutoArchive(ctx context.Context, archived []database.AutoArchiveInactiveChatsRow) {
	auditor := *i.auditor.Load()
	for _, row := range archived {
		if row.ParentChatID.Valid {
			continue // Children inherit root's archival; audit roots only.
		}
		after := chatFromAutoArchiveRow(i.logger, row)
		before := after
		before.Archived = false
		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.Chat]{
			Audit:            auditor,
			Log:              i.logger,
			UserID:           row.OwnerID,
			OrganizationID:   row.OrganizationID,
			Action:           database.AuditActionWrite,
			Old:              before,
			New:              after,
			Status:           http.StatusOK,
			AdditionalFields: audit.BackgroundTaskFieldsBytes(ctx, i.logger, audit.BackgroundSubsystemChatAutoArchive),
		})
	}
}
