package gitsync

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

const (
	// defaultBatchSize is the maximum number of stale rows fetched
	// per tick.
	defaultBatchSize int32 = 50

	// defaultInterval is the polling interval between ticks.
	defaultInterval = 10 * time.Second
)

// Store is the narrow DB interface the Worker needs.
type Store interface {
	AcquireStaleChatDiffStatuses(
		ctx context.Context, limitVal int32,
	) ([]database.AcquireStaleChatDiffStatusesRow, error)
	BackoffChatDiffStatus(
		ctx context.Context, arg database.BackoffChatDiffStatusParams,
	) error
	UpsertChatDiffStatus(
		ctx context.Context, arg database.UpsertChatDiffStatusParams,
	) (database.ChatDiffStatus, error)
	UpsertChatDiffStatusReference(
		ctx context.Context, arg database.UpsertChatDiffStatusReferenceParams,
	) (database.ChatDiffStatus, error)
	GetChatsByOwnerID(
		ctx context.Context, arg database.GetChatsByOwnerIDParams,
	) ([]database.Chat, error)
}

// EventPublisher notifies the frontend of diff status changes.
type PublishDiffStatusChangeFunc func(ctx context.Context, chatID uuid.UUID) error

// Worker is a background loop that periodically refreshes stale
// chat diff statuses by delegating to a Refresher.
type Worker struct {
	store                     Store
	refresher                 *Refresher
	publishDiffStatusChangeFn PublishDiffStatusChangeFunc
	clock                     quartz.Clock
	logger                    slog.Logger
	batchSize                 int32
	interval                  time.Duration
	done                      chan struct{}
}

// NewWorker creates a Worker with default batch size and interval.
func NewWorker(
	store Store,
	refresher *Refresher,
	publisher PublishDiffStatusChangeFunc,
	clock quartz.Clock,
	logger slog.Logger,
) *Worker {
	return &Worker{
		store:                     store,
		refresher:                 refresher,
		publishDiffStatusChangeFn: publisher,
		clock:                     clock,
		logger:                    logger,
		batchSize:                 defaultBatchSize,
		interval:                  defaultInterval,
		done:                      make(chan struct{}),
	}
}

// Start launches the background loop. It blocks until ctx is
// cancelled, then closes w.done.
func (w *Worker) Start(ctx context.Context) {
	defer close(w.done)

	ticker := w.clock.NewTicker(w.interval, "gitsync", "worker")
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// Done returns a channel that is closed when the worker exits.
func (w *Worker) Done() <-chan struct{} {
	return w.done
}

func chatDiffStatusFromRow(row database.AcquireStaleChatDiffStatusesRow) database.ChatDiffStatus {
	return database.ChatDiffStatus{
		ChatID:           row.ChatID,
		Url:              row.Url,
		PullRequestState: row.PullRequestState,
		ChangesRequested: row.ChangesRequested,
		Additions:        row.Additions,
		Deletions:        row.Deletions,
		ChangedFiles:     row.ChangedFiles,
		RefreshedAt:      row.RefreshedAt,
		StaleAt:          row.StaleAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		GitBranch:        row.GitBranch,
		GitRemoteOrigin:  row.GitRemoteOrigin,
	}
}

func (w *Worker) tick(ctx context.Context) {
	// Set a context equal to w.interval so that we do not hold up processing due to
	// random unicorn-related events.
	ctx, cancel := context.WithTimeout(ctx, w.interval)
	defer cancel()

	acquiredRows, err := w.store.AcquireStaleChatDiffStatuses(ctx, w.batchSize)
	if err != nil {
		w.logger.Warn(ctx, "acquire stale chat diff statuses",
			slog.Error(err))
		return
	}
	if len(acquiredRows) == 0 {
		return
	}

	// Build refresh requests directly from acquired rows.
	requests := make([]RefreshRequest, 0, len(acquiredRows))
	for _, row := range acquiredRows {
		requests = append(requests, RefreshRequest{
			Row:     chatDiffStatusFromRow(row),
			OwnerID: row.OwnerID,
		})
	}

	results, err := w.refresher.Refresh(ctx, requests)
	if err != nil {
		w.logger.Warn(ctx, "batch refresh chat diff statuses",
			slog.Error(err))
		return
	}

	for _, res := range results {
		if res.Error != nil {
			w.logger.Debug(ctx, "refresh chat diff status",
				slog.F("chat_id", res.Request.Row.ChatID),
				slog.Error(res.Error))
			// Back off so the row isn't retried immediately.
			if err := w.store.BackoffChatDiffStatus(ctx,
				database.BackoffChatDiffStatusParams{
					ChatID:  res.Request.Row.ChatID,
					StaleAt: w.clock.Now().UTC().Add(DiffStatusTTL),
				},
			); err != nil {
				w.logger.Warn(ctx, "backoff failed chat diff status",
					slog.F("chat_id", res.Request.Row.ChatID),
					slog.Error(err))
			}
			continue
		}
		if res.Params == nil {
			// No PR yet — skip.
			continue
		}
		if _, err := w.store.UpsertChatDiffStatus(ctx, *res.Params); err != nil {
			w.logger.Warn(ctx, "upsert refreshed chat diff status",
				slog.F("chat_id", res.Request.Row.ChatID),
				slog.Error(err))
			continue
		}
		if w.publishDiffStatusChangeFn != nil {
			if err := w.publishDiffStatusChangeFn(ctx, res.Request.Row.ChatID); err != nil {
				w.logger.Debug(ctx, "publish diff status change",
					slog.F("chat_id", res.Request.Row.ChatID),
					slog.Error(err))
			}
		}
	}
}

// MarkStale persists the git ref on all chats for a workspace,
// setting stale_at to the past so the next tick picks them up.
// Publishes a diff status event for each affected chat.
// Called from workspaceagents handlers. No goroutines spawned.
func (w *Worker) MarkStale(
	ctx context.Context,
	workspaceID, ownerID uuid.UUID,
	branch, origin string,
) {
	if branch == "" || origin == "" {
		return
	}

	chats, err := w.store.GetChatsByOwnerID(ctx, database.GetChatsByOwnerIDParams{
		OwnerID: ownerID,
	})
	if err != nil {
		w.logger.Warn(ctx, "list chats for git ref storage",
			slog.F("workspace_id", workspaceID),
			slog.Error(err))
		return
	}

	for _, chat := range filterChatsByWorkspaceID(chats, workspaceID) {
		_, err := w.store.UpsertChatDiffStatusReference(ctx,
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				GitBranch:       branch,
				GitRemoteOrigin: origin,
				StaleAt:         w.clock.Now().Add(-time.Second),
				Url:             sql.NullString{},
			},
		)
		if err != nil {
			w.logger.Warn(ctx, "store git ref on chat diff status",
				slog.F("chat_id", chat.ID),
				slog.F("workspace_id", workspaceID),
				slog.Error(err))
			continue
		}
		// Notify the frontend immediately so the UI shows the
		// branch info even before the worker refreshes PR data.
		if w.publishDiffStatusChangeFn != nil {
			if pubErr := w.publishDiffStatusChangeFn(ctx, chat.ID); pubErr != nil {
				w.logger.Debug(ctx, "publish diff status after mark stale",
					slog.F("chat_id", chat.ID), slog.Error(pubErr))
			}
		}
	}
}

// filterChatsByWorkspaceID returns only chats associated with
// the given workspace.
func filterChatsByWorkspaceID(
	chats []database.Chat,
	workspaceID uuid.UUID,
) []database.Chat {
	filtered := make([]database.Chat, 0, len(chats))
	for _, chat := range chats {
		if !chat.WorkspaceID.Valid || chat.WorkspaceID.UUID != workspaceID {
			continue
		}
		filtered = append(filtered, chat)
	}
	return filtered
}
