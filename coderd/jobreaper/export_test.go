package jobreaper

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// ReapPendingJob runs the locked recheck and reap for a pending job, as the
// detector does for a candidate job.
func ReapPendingJob(ctx context.Context, log slog.Logger, db database.Store, pub pubsub.Pubsub, id uuid.UUID) error {
	return reapJob(ctx, log, db, pub, &jobToReap{ID: id, Type: Pending, Threshold: PendingJobDuration})
}
