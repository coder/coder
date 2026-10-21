package chatd

import (
	"context"

	"cdr.dev/slog/v3"
)

// deletedChatProjectsLoop archives the chats of deleted projects whose
// cleanup did not run or failed, for example because the replica that
// handled the delete stopped or the cleanup deadlocked.
func (w *chatWorker) deletedChatProjectsLoop(ctx context.Context) {
	ticker := w.opts.Clock.NewTicker(w.opts.DeletedChatProjectSweepInterval, "chatworker", "deleted-chat-projects")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		w.sweepDeletedChatProjects(ctx)
	}
}

func (w *chatWorker) sweepDeletedChatProjects(ctx context.Context) {
	ids, err := w.opts.Store.GetDeletedChatProjectIDsWithUnarchivedChats(ctx, defaultDeletedChatProjectSweepBatchSize)
	if err != nil {
		if ctx.Err() == nil {
			w.opts.Logger.Warn(ctx, "chatworker deleted chat project query failed", slogError(err))
		}
		return
	}
	for _, id := range ids {
		if err := w.server.cleanupDeletedChatProject(ctx, id); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.opts.Logger.Warn(ctx, "chatworker deleted chat project cleanup failed; retrying next tick",
				slog.F("project_id", id), slogError(err))
		}
	}
}
