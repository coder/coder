package boundaryusage

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// Tracker tracks boundary usage for telemetry reporting.
//
// All stats accumulate in memory throughout a telemetry period and are only
// reset when a new period begins.
type Tracker struct {
	mu              sync.Mutex
	workspaces      map[uuid.UUID]struct{}
	users           map[uuid.UUID]struct{}
	allowedRequests int64
	deniedRequests  int64
}

// NewTracker creates a new boundary usage tracker.
func NewTracker() *Tracker {
	return &Tracker{
		workspaces: make(map[uuid.UUID]struct{}),
		users:      make(map[uuid.UUID]struct{}),
	}
}

// Track records boundary usage for a workspace.
func (t *Tracker) Track(workspaceID, ownerID uuid.UUID, allowed, denied int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.workspaces[workspaceID] = struct{}{}
	t.users[ownerID] = struct{}{}
	t.allowedRequests += allowed
	t.deniedRequests += denied
}

// FlushToDB writes the accumulated stats to the database. All values are
// replaced in the database (they represent the current in-memory state). If the
// database row was deleted (new telemetry period), all in-memory stats are reset.
func (t *Tracker) FlushToDB(ctx context.Context, db database.Store, replicaID uuid.UUID) error {
	t.mu.Lock()
	workspaceCount := int64(len(t.workspaces))
	userCount := int64(len(t.users))
	allowed := t.allowedRequests
	denied := t.deniedRequests
	t.mu.Unlock()

	// Don't flush if there's no activity.
	if workspaceCount == 0 && userCount == 0 && allowed == 0 && denied == 0 {
		return nil
	}

	//nolint:gocritic // This is the actual package doing boundary usage tracking.
	newPeriod, err := db.UpsertBoundaryUsageStats(dbauthz.AsBoundaryUsageTracker(ctx), database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesCount: workspaceCount,
		UniqueUsersCount:      userCount,
		AllowedRequests:       allowed,
		DeniedRequests:        denied,
	})
	if err != nil {
		return err
	}

	// If this was an insert (new period), reset all stats. Any Track() calls
	// that occurred during the DB operation will be counted in the next period.
	if newPeriod {
		t.mu.Lock()
		t.workspaces = make(map[uuid.UUID]struct{})
		t.users = make(map[uuid.UUID]struct{})
		t.allowedRequests = 0
		t.deniedRequests = 0
		t.mu.Unlock()
	}

	return nil
}

// StartFlushLoop begins the periodic flush loop that writes accumulated stats
// to the database. It blocks until the context is canceled. Flushes every
// minute to keep stats reasonably fresh for telemetry collection (which runs
// every 30 minutes by default) without excessive DB writes.
func (t *Tracker) StartFlushLoop(ctx context.Context, log slog.Logger, db database.Store, replicaID uuid.UUID) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.FlushToDB(ctx, db, replicaID); err != nil {
				log.Warn(ctx, "failed to flush boundary usage stats", slog.Error(err))
			}
		}
	}
}
