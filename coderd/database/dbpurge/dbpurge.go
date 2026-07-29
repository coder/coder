package dbpurge

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
	// Batch size for boundary log deletion.
	boundaryLogsBatchSize = 10000
	// Batch size for boundary session deletion.
	boundarySessionsBatchSize = 10000
	// Telemetry heartbeats are used to deduplicate events across replicas. We
	// don't need to persist heartbeat rows for longer than 24 hours, as they
	// are only used for deduplication across replicas. The time needs to be
	// long enough to cover the maximum interval of a heartbeat event (currently
	// 1 hour) plus some buffer.
	maxTelemetryHeartbeatAge = 24 * time.Hour
	// Operational handoff state; terminal rows are kept for debugging, then
	// purged.
	workspaceBuildOrchestrationTerminalRetention = 24 * time.Hour
	// Batch size for workspace build orchestration deletion.
	workspaceBuildOrchestrationsBatchSize = 10000
	// Chat and chat file batch sizes stay smaller than audit/connection
	// log batches because chat_files rows carry bytea blobs.
	chatsBatchSize     = 1000
	chatFilesBatchSize = 1000
	// Chat debug run deletions can cascade into steps with large JSONB
	// payloads, so they use the same conservative batch size.
	chatDebugRunsBatchSize = 1000
	// Chat search tsvector backfill is capped at 5 batches of 10k
	// rows per tick. Benchmarks on a dogfood-class machine (EPYC 9454P)
	// with containerized Postgres were measured to take ~800ms per batch.
	// This is considered acceptable but may need dialing in later.
	chatSearchBackfillBatchSize  = 10000
	chatSearchBackfillMaxBatches = 5
)

type Option func(*instance)

// WithClock overrides the clock used by the purger. Defaults to
// quartz.NewReal().
func WithClock(clk quartz.Clock) Option {
	return func(i *instance) { i.clk = clk }
}

// WithChatSearchBackfillLimits overrides backfill batch size and cap. For tests.
func WithChatSearchBackfillLimits(batchSize int32, maxBatches int) Option {
	return func(i *instance) {
		i.chatSearchBackfillBatchSize = batchSize
		i.chatSearchBackfillMaxBatches = maxBatches
	}
}

// New creates a new periodically purging database instance.
// Callers must Close the returned instance.
func New(ctx context.Context, logger slog.Logger, db database.Store, vals *codersdk.DeploymentValues, reg prometheus.Registerer, opts ...Option) io.Closer {
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

	chatSearchRowsBackfilled := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "dbpurge",
		Name:      "chat_search_rows_backfilled_total",
		Help:      "Total number of chat message rows whose search_tsv was backfilled.",
	})
	reg.MustRegister(chatSearchRowsBackfilled)

	inst := &instance{
		cancel:                       cancelFunc,
		closed:                       closed,
		logger:                       logger,
		vals:                         vals,
		clk:                          quartz.NewReal(),
		iterationDuration:            iterationDuration,
		recordsPurged:                recordsPurged,
		chatSearchRowsBackfilled:     chatSearchRowsBackfilled,
		chatSearchBackfillBatchSize:  chatSearchBackfillBatchSize,
		chatSearchBackfillMaxBatches: chatSearchBackfillMaxBatches,
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
	// Read chat configs outside the tx so a corrupt value can't
	// poison subsequent queries. On config read errors, log and stash
	// the error, then run unrelated purges best-effort. Retention
	// errors skip only the conversation purge. Debug retention errors
	// skip only the debug purge. purgeTick returns chatConfigErr after
	// the tx so the failed iteration is operator-visible via metric and
	// logs.
	chatRetentionDays, chatRetentionErr := db.GetChatRetentionDays(ctx)
	purgeChats := chatRetentionErr == nil
	if chatRetentionErr != nil {
		i.logger.Error(ctx, "failed to read chat retention config: skipping chat purge this tick", slog.Error(chatRetentionErr))
	}

	chatDebugRetentionDays, chatDebugRetentionErr := db.GetChatDebugRetentionDays(ctx, codersdk.DefaultChatDebugRetentionDays)
	purgeChatDebugRuns := chatDebugRetentionErr == nil
	if chatDebugRetentionErr != nil {
		i.logger.Error(ctx, "failed to read chat debug retention config: skipping chat debug purge this tick", slog.Error(chatDebugRetentionErr))
	}

	chatConfigErr := errors.Join(chatRetentionErr, chatDebugRetentionErr)

	// Start a transaction to grab advisory lock, we don't want to run
	// multiple purges at the same time (multiple replicas).
	err := db.InTx(func(tx database.Store) error {
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

		var purgedBoundaryLogs, purgedBoundarySessions int64
		boundaryLogsRetention := i.vals.Retention.BoundaryLogs.Value()
		if boundaryLogsRetention > 0 {
			deleteBoundaryLogsBefore := start.Add(-boundaryLogsRetention)
			purgedBoundaryLogs, err = tx.DeleteOldBoundaryLogs(ctx, database.DeleteOldBoundaryLogsParams{
				BeforeTime: deleteBoundaryLogsBefore,
				LimitCount: boundaryLogsBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old boundary logs: %w", err)
			}
			purgedBoundarySessions, err = tx.DeleteOldBoundarySessions(ctx, database.DeleteOldBoundarySessionsParams{
				BeforeTime: deleteBoundaryLogsBefore,
				LimitCount: boundarySessionsBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old boundary sessions: %w", err)
			}
		}

		deleteOldWorkspaceBuildOrchestrationsBefore := start.Add(-workspaceBuildOrchestrationTerminalRetention)
		purgedWorkspaceBuildOrchestrations, err := tx.DeleteOldWorkspaceBuildOrchestrations(ctx, database.DeleteOldWorkspaceBuildOrchestrationsParams{
			BeforeTime: deleteOldWorkspaceBuildOrchestrationsBefore,
			LimitCount: workspaceBuildOrchestrationsBatchSize,
		})
		if err != nil {
			return xerrors.Errorf("failed to delete old workspace build orchestrations: %w", err)
		}

		var purgedChats, purgedChatFiles, purgedChatDebugRuns int64
		if purgeChats {
			purgedChats, purgedChatFiles, err = i.purgeChatsInTx(ctx, tx, start, chatRetentionDays)
			if err != nil {
				return xerrors.Errorf("failed to purge chats: %w", err)
			}
		}
		if purgeChatDebugRuns && chatDebugRetentionDays > 0 {
			deleteChatDebugRunsBefore := start.Add(-time.Duration(chatDebugRetentionDays) * 24 * time.Hour)
			// updated_at is the retention clock, so the window starts after
			// the run stops being written to. There is intentionally no
			// finished_at guard, so abandoned in-flight rows can be purged.
			purgedChatDebugRuns, err = tx.DeleteOldChatDebugRuns(ctx, database.DeleteOldChatDebugRunsParams{
				BeforeTime: deleteChatDebugRunsBefore,
				LimitCount: chatDebugRunsBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("failed to delete old chat debug runs: %w", err)
			}
		}

		// Backfill search_tsv tsvector on chat_messages in batches. Doing this here because it's
		// potentially too much for a regular migration, especially on larger deployments:
		// - Each row with search_tsv = NULL is present in idx_chat_messages_search_tsv_pending.
		// - Content of chat_messages is not changed after insert.
		// - Rows that are soft-deleted are no longer part of the index.
		// NOTE: This should not remain in dbpurge and should be adjusted when the "DBOps" gets
		//   implemented.
		var backfilledChatSearchRows int64
		for range i.chatSearchBackfillMaxBatches {
			n, err := tx.BackfillChatMessagesSearchTsv(ctx, i.chatSearchBackfillBatchSize)
			if err != nil {
				return xerrors.Errorf("backfill chat_messages.search_tsv: %w", err)
			}
			backfilledChatSearchRows += n
			if n < int64(i.chatSearchBackfillBatchSize) {
				break
			}
		}

		i.logger.Debug(ctx, "purged old database entries",
			slog.F("workspace_agent_logs", purgedWorkspaceAgentLogs),
			slog.F("expired_api_keys", expiredAPIKeys),
			slog.F("aibridge_records", purgedAIBridgeRecords),
			slog.F("connection_logs", purgedConnectionLogs),
			slog.F("audit_logs", purgedAuditLogs),
			slog.F("boundary_logs", purgedBoundaryLogs),
			slog.F("boundary_sessions", purgedBoundarySessions),
			slog.F("workspace_build_orchestrations", purgedWorkspaceBuildOrchestrations),
			slog.F("chats", purgedChats),
			slog.F("chat_files", purgedChatFiles),
			slog.F("chat_debug_runs", purgedChatDebugRuns),
			slog.F("chat_search_rows_backfilled", backfilledChatSearchRows),
			slog.F("duration", i.clk.Since(start)),
		)

		if i.recordsPurged != nil {
			i.recordsPurged.WithLabelValues("workspace_agent_logs").Add(float64(purgedWorkspaceAgentLogs))
			i.recordsPurged.WithLabelValues("expired_api_keys").Add(float64(expiredAPIKeys))
			i.recordsPurged.WithLabelValues("aibridge_records").Add(float64(purgedAIBridgeRecords))
			i.recordsPurged.WithLabelValues("connection_logs").Add(float64(purgedConnectionLogs))
			i.recordsPurged.WithLabelValues("audit_logs").Add(float64(purgedAuditLogs))
			i.recordsPurged.WithLabelValues("boundary_logs").Add(float64(purgedBoundaryLogs))
			i.recordsPurged.WithLabelValues("boundary_sessions").Add(float64(purgedBoundarySessions))
			i.recordsPurged.WithLabelValues("workspace_build_orchestrations").Add(float64(purgedWorkspaceBuildOrchestrations))
			i.recordsPurged.WithLabelValues("chats").Add(float64(purgedChats))
			i.recordsPurged.WithLabelValues("chat_debug_runs").Add(float64(purgedChatDebugRuns))
			i.recordsPurged.WithLabelValues("chat_files").Add(float64(purgedChatFiles))
		}
		if i.chatSearchRowsBackfilled != nil {
			i.chatSearchRowsBackfilled.Add(float64(backfilledChatSearchRows))
		}

		// chatConfigErr is returned after the tx, so do not record this
		// iteration as successful when only the deferred config read failed.
		if i.iterationDuration != nil && chatConfigErr == nil {
			duration := i.clk.Since(start)
			i.iterationDuration.WithLabelValues("true").Observe(duration.Seconds())
		}

		return nil
	}, database.DefaultTXOptions().WithID("db_purge"))
	if err != nil {
		return err
	}

	// Surface the deferred chat-config error so doTick records
	// the failed iteration metric.
	if chatConfigErr != nil {
		return xerrors.Errorf("chat config read failed this tick: %w", chatConfigErr)
	}

	return nil
}

type instance struct {
	cancel                       context.CancelFunc
	closed                       chan struct{}
	logger                       slog.Logger
	vals                         *codersdk.DeploymentValues
	clk                          quartz.Clock
	iterationDuration            *prometheus.HistogramVec
	recordsPurged                *prometheus.CounterVec
	chatSearchRowsBackfilled     prometheus.Counter
	chatSearchBackfillBatchSize  int32
	chatSearchBackfillMaxBatches int
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}

// purgeChatsInTx MUST BE CALLED WITH A TRANSACTION
func (*instance) purgeChatsInTx(ctx context.Context, tx database.Store, start time.Time, chatRetentionDays int32) (purgedChats, purgedChatFiles int64, err error) {
	// Delete old archived chats first, then orphaned files
	// (cascade clears chat_file_links but not chat_files).
	if chatRetentionDays > 0 {
		deleteChatsBefore := start.Add(-time.Duration(chatRetentionDays) * 24 * time.Hour)
		purgedChats, err = tx.DeleteOldChats(ctx, database.DeleteOldChatsParams{
			BeforeTime: deleteChatsBefore,
			LimitCount: chatsBatchSize,
		})
		if err != nil {
			return 0, 0, xerrors.Errorf("failed to delete old chats: %w", err)
		}

		purgedChatFiles, err = tx.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
			BeforeTime: deleteChatsBefore,
			LimitCount: chatFilesBatchSize,
		})
		if err != nil {
			return 0, 0, xerrors.Errorf("failed to delete old chat files: %w", err)
		}
	}

	return purgedChats, purgedChatFiles, nil
}
