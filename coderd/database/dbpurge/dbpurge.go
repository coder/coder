package dbpurge

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/util/slice"
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
	// chatAutoArchiveDigestMaxChats bounds how many chat titles a
	// single digest body lists. Past the cap, surplus titles are
	// summarized as "...and N more". 25 is a readable email-friendly
	// length; the cap is unrelated to chatAutoArchiveBatchSize, which
	// bounds work per tick.
	chatAutoArchiveDigestMaxChats = 25
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

// WithNotificationsEnqueuer sets the enqueuer used for digest
// notifications. Defaults to notifications.NewNoopEnqueuer(). Panics
// if e is nil: a nil enqueuer would NPE on the first dispatch tick,
// and failing fast at option-apply time surfaces the misuse at
// startup rather than minutes later.
func WithNotificationsEnqueuer(e notifications.Enqueuer) Option {
	if e == nil {
		panic("developer error: WithNotificationsEnqueuer called with nil enqueuer")
	}
	return func(i *instance) { i.enqueuer = e }
}

// New creates a new periodically purging database instance.
// Callers must Close the returned instance.
//
// The auditor pointer is loaded on each dispatch tick so runtime
// entitlement changes (e.g. toggling the audit-log feature) take
// effect without restarting the process. Notifications enqueuer
// defaults to no-op. Use WithNotificationsEnqueuer to pass a real
// one.
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
		enqueuer:                 notifications.NewNoopEnqueuer(),
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
	// Read chat configs outside the tx so a corrupt value can't
	// poison subsequent queries. On error we log and stash, then
	// run unrelated purges best-effort and skip only chat work;
	// purgeTick returns chatConfigErr after the tx so the failed
	// iteration is operator-visible via metric and logs.
	chatRetentionDays, chatRetentionErr := db.GetChatRetentionDays(ctx)
	if chatRetentionErr != nil {
		i.logger.Error(ctx, "failed to read chat retention config: skipping chat purge and auto-archive this tick", slog.Error(chatRetentionErr))
	}

	chatAutoArchiveDays, chatAutoArchiveErr := db.GetChatAutoArchiveDays(ctx, codersdk.DefaultChatAutoArchiveDays)
	if chatAutoArchiveErr != nil {
		i.logger.Error(ctx, "failed to read chat auto-archive config: skipping chat purge and auto-archive this tick", slog.Error(chatAutoArchiveErr))
	}

	chatConfigErr := errors.Join(chatRetentionErr, chatAutoArchiveErr)

	// Populated inside the tx; dispatched post-commit.
	var archivedChats []database.AutoArchiveInactiveChatsRow

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

		var purgedChats, purgedChatFiles int64
		if chatConfigErr == nil {
			purgedChats, purgedChatFiles, archivedChats, err = i.purgeChatsInTx(ctx, tx, start, chatRetentionDays, chatAutoArchiveDays)
			if err != nil {
				return xerrors.Errorf("failed to purge chats: %w", err)
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

	// Surface the deferred chat-config error so doTick records
	// the failed iteration metric.
	if chatConfigErr != nil {
		return xerrors.Errorf("chat config read failed this tick: %w", chatConfigErr)
	}

	// Dispatch audits and digests post-commit. Detached context for audit
	// so that ticker cancellation cannot truncate the audit trail.
	// Notification enqueue uses the cancellable parent context to avoid
	// stalling shutdown.
	// Owners with more eligible chats than batch size will get a
	// notification per tick until their backlog drains.
	// If this is deemed too noisy, users can disable the
	// "Chats Auto-Archived" template from their notification preferences.
	if len(archivedChats) > 0 {
		i.chatAutoArchiveRecords.Add(float64(len(archivedChats)))
		auditCtx := context.WithoutCancel(ctx)
		i.dispatchChatAutoArchive(auditCtx, ctx, start, chatAutoArchiveDays, chatRetentionDays, archivedChats)
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
	enqueuer                 notifications.Enqueuer
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

// purgeChatsInTx MUST BE CALLED WITH A TRANSACTION
func (i *instance) purgeChatsInTx(ctx context.Context, tx database.Store, start time.Time, chatRetentionDays, chatAutoArchiveDays int32) (purgedChats, purgedChatFiles int64, archivedChats []database.AutoArchiveInactiveChatsRow, err error) {
	// Delete old archived chats first, then orphaned files
	// (cascade clears chat_file_links but not chat_files).
	if chatRetentionDays > 0 {
		deleteChatsBefore := start.Add(-time.Duration(chatRetentionDays) * 24 * time.Hour)
		purgedChats, err = tx.DeleteOldChats(ctx, database.DeleteOldChatsParams{
			BeforeTime: deleteChatsBefore,
			LimitCount: chatsBatchSize,
		})
		if err != nil {
			return 0, 0, nil, xerrors.Errorf("failed to delete old chats: %w", err)
		}

		purgedChatFiles, err = tx.DeleteOldChatFiles(ctx, database.DeleteOldChatFilesParams{
			BeforeTime: deleteChatsBefore,
			LimitCount: chatFilesBatchSize,
		})
		if err != nil {
			return 0, 0, nil, xerrors.Errorf("failed to delete old chat files: %w", err)
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
			return 0, 0, nil, xerrors.Errorf("failed to auto-archive inactive chats: %w", err)
		}
	}
	return purgedChats, purgedChatFiles, archivedChats, nil
}

// dispatchChatAutoArchive audits every archived root chat and enqueues one
// notification per owner covering the roots archived in this tick. Children
// inherit their root's archival decision and are skipped for audit, matching
// the manual archive path (patchChat audits the root only). Enqueue is
// per-tick: owners whose backlog spans multiple ticks receive multiple
// notifications; notification_messages dedupe does not collapse them because
// each tick's payload differs.
//
// auditCtx is detached from the ticker so audits always complete. enqueueCtx
// is the cancellable parent: on shutdown we abandon any remaining digests
// rather than blocking Close.
func (i *instance) dispatchChatAutoArchive(auditCtx, enqueueCtx context.Context, tickStart time.Time, autoArchiveDays, retentionDays int32, archived []database.AutoArchiveInactiveChatsRow) {
	// Children inherit their root's archival decision and are skipped
	// for both audit and digest. Partition once so the two loops
	// cannot drift apart if the cascade shape ever changes.
	roots := slice.Filter(archived, func(r database.AutoArchiveInactiveChatsRow) bool {
		return !r.ParentChatID.Valid
	})

	auditor := *i.auditor.Load()
	for _, row := range roots {
		after := chatFromAutoArchiveRow(i.logger, row)
		before := after
		before.Archived = false
		audit.BackgroundAudit(auditCtx, &audit.BackgroundAuditParams[database.Chat]{
			Audit:            auditor,
			Log:              i.logger,
			UserID:           row.OwnerID,
			OrganizationID:   row.OrganizationID,
			Action:           database.AuditActionWrite,
			Old:              before,
			New:              after,
			Status:           http.StatusOK,
			AdditionalFields: audit.BackgroundTaskFieldsBytes(auditCtx, i.logger, audit.BackgroundSubsystemChatAutoArchive),
		})
	}

	// Group archived roots by owner. Inline because this is the
	// only call site and the loop body is self-explanatory.
	rootsByOwner := make(map[uuid.UUID][]database.AutoArchiveInactiveChatsRow, len(roots))
	for _, row := range roots {
		rootsByOwner[row.OwnerID] = append(rootsByOwner[row.OwnerID], row)
	}

	// Sort owner IDs so shutdown abandons a deterministic tail of the dispatch list.
	ownerIDs := make([]uuid.UUID, 0, len(rootsByOwner))
	for id := range rootsByOwner {
		ownerIDs = append(ownerIDs, id)
	}
	slices.SortFunc(ownerIDs, func(a, b uuid.UUID) int {
		return cmp.Compare(a.String(), b.String())
	})

	dispatched := 0
	for _, ownerID := range ownerIDs {
		// Check between iterations so shutdown unblocks promptly. A
		// hung in-flight enqueue is unblocked by enqueueCtx propagating
		// cancellation into the DB call. Skipped owners are not
		// re-notified on the next tick because AutoArchiveInactiveChats
		// only returns rows with archived = false; we accept that
		// tradeoff over hanging shutdown.
		if err := enqueueCtx.Err(); err != nil {
			i.logger.Warn(enqueueCtx, "chat auto-archive digest dispatch canceled",
				slog.F("remaining_owners", len(ownerIDs)-dispatched),
				slog.Error(err))
			return
		}
		dispatched++

		ownerRoots := rootsByOwner[ownerID]
		data := buildDigestData(ownerRoots, autoArchiveDays, retentionDays, tickStart)

		// nolint:gocritic // Background digest runs as the notifier subject.
		if _, err := i.enqueuer.EnqueueWithData(
			dbauthz.AsNotifier(enqueueCtx),
			ownerID,
			notifications.TemplateChatAutoArchiveDigest,
			map[string]string{},
			data,
			string(audit.BackgroundSubsystemChatAutoArchive),
		); err != nil {
			i.logger.Warn(enqueueCtx, "failed to enqueue chat auto-archive digest",
				slog.F("owner_id", ownerID),
				slog.Error(err))
		}
	}
}

// buildDigestData builds the notification payload; shape mirrors the
// golden fixtures in coderd/notifications/testdata. Truncation keeps
// the oldest archived roots (created_at ASC from the query) to
// preserve index-driven ordering; revisit if the digest becomes the
// primary surface for reviewing archived chats.
func buildDigestData(rows []database.AutoArchiveInactiveChatsRow, autoArchiveDays, retentionDays int32, tickStart time.Time) map[string]any {
	// Cap titles; overflow surfaces as "...and N more" via the template.
	overflow := 0
	if len(rows) > chatAutoArchiveDigestMaxChats {
		overflow = len(rows) - chatAutoArchiveDigestMaxChats
		rows = rows[:chatAutoArchiveDigestMaxChats]
	}

	chats := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		chats = append(chats, map[string]any{
			"title":                   r.Title,
			"last_activity_humanized": humanize.RelTime(r.LastActivityAt, tickStart, "ago", "from now"),
		})
	}

	// Stringify the int32 config values: the template's
	// {{if eq .Data.retention_days "0"}} branch requires both
	// operands to share a type, and Go templates do not coerce
	// numeric ↔ string. Storing a raw int here would silently
	// take the deletion-warning branch on every notification.
	data := map[string]any{
		"auto_archive_days": strconv.Itoa(int(autoArchiveDays)),
		"retention_days":    strconv.Itoa(int(retentionDays)),
		"archived_chats":    chats,
	}
	if overflow > 0 {
		data["additional_archived_count"] = strconv.Itoa(overflow)
	}
	return data
}
