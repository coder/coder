package boundaryusage

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// Tracker tracks boundary usage for telemetry reporting.
//
// All stats accumulate in memory throughout a telemetry period and are only
// reset when a new period begins (detected when the database row is deleted
// by telemetry).
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
	uniqueWorkspaces := int64(len(t.workspaces))
	uniqueUsers := int64(len(t.users))
	allowedRequests := t.allowedRequests
	deniedRequests := t.deniedRequests
	t.mu.Unlock()

	// Nothing to flush if no activity.
	if uniqueWorkspaces == 0 && uniqueUsers == 0 && allowedRequests == 0 && deniedRequests == 0 {
		return nil
	}

	newPeriod, err := db.UpsertBoundaryUsageStats(ctx, database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesCount: uniqueWorkspaces,
		UniqueUsersCount:      uniqueUsers,
		AllowedRequests:       allowedRequests,
		DeniedRequests:        deniedRequests,
	})
	if err != nil {
		return err
	}

	// If this was an INSERT (new period), reset all stats. This happens when
	// telemetry deleted our row, signaling the start of a new period.
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
