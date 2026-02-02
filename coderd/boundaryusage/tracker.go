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
// Unique user/workspace counts are tracked both cumulatively and as deltas since
// the last flush. The delta is needed because when a new telemetry period starts
// (the DB row is deleted), we must only insert data accumulated since the last
// flush. If we used cumulative values, stale data from the previous period would
// be written to the new row and then lost when subsequent updates overwrite it.
//
// Request counts are tracked as deltas and accumulated in the database.
type Tracker struct {
	mu sync.Mutex

	// Cumulative unique counts for the current period (used on UPDATE to
	// replace the DB value with accurate totals).
	workspaces map[uuid.UUID]struct{}
	users      map[uuid.UUID]struct{}

	// Delta unique counts since last flush (used on INSERT to avoid writing
	// stale data from the previous period).
	workspacesDelta map[uuid.UUID]struct{}
	usersDelta      map[uuid.UUID]struct{}

	// Request deltas (always reset when flushing, accumulated in DB).
	allowedRequests int64
	deniedRequests  int64

	usageSinceLastFlush bool
}

// NewTracker creates a new boundary usage tracker.
func NewTracker() *Tracker {
	return &Tracker{
		workspaces:      make(map[uuid.UUID]struct{}),
		users:           make(map[uuid.UUID]struct{}),
		workspacesDelta: make(map[uuid.UUID]struct{}),
		usersDelta:      make(map[uuid.UUID]struct{}),
	}
}

// Track records boundary usage for a workspace.
func (t *Tracker) Track(workspaceID, ownerID uuid.UUID, allowed, denied int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.workspaces[workspaceID] = struct{}{}
	t.users[ownerID] = struct{}{}
	t.workspacesDelta[workspaceID] = struct{}{}
	t.usersDelta[ownerID] = struct{}{}
	t.allowedRequests += allowed
	t.deniedRequests += denied
	t.usageSinceLastFlush = true
}

// FlushToDB writes stats to the database. For unique counts, cumulative values
// are used on UPDATE (replacing the DB value) while delta values are used on
// INSERT (starting fresh). Request counts are always deltas, accumulated in DB.
// All deltas are reset immediately after snapshot so Track() calls during the
// DB operation are preserved for the next flush.
func (t *Tracker) FlushToDB(ctx context.Context, db database.Store, replicaID uuid.UUID) error {
	t.mu.Lock()
	if !t.usageSinceLastFlush {
		t.mu.Unlock()
		return nil
	}

	// Snapshot all values.
	workspaceCount := int64(len(t.workspaces))      // cumulative, for UPDATE
	userCount := int64(len(t.users))                // cumulative, for UPDATE
	workspaceDelta := int64(len(t.workspacesDelta)) // delta, for INSERT
	userDelta := int64(len(t.usersDelta))           // delta, for INSERT
	allowed := t.allowedRequests                    // delta, accumulated in DB
	denied := t.deniedRequests                      // delta, accumulated in DB

	// Reset all deltas immediately so Track() calls during the DB operation
	// below are preserved for the next flush.
	t.workspacesDelta = make(map[uuid.UUID]struct{})
	t.usersDelta = make(map[uuid.UUID]struct{})
	t.allowedRequests = 0
	t.deniedRequests = 0
	t.usageSinceLastFlush = false
	t.mu.Unlock()

	//nolint:gocritic // This is the actual package doing boundary usage tracking.
	_, err := db.UpsertBoundaryUsageStats(dbauthz.AsBoundaryUsageTracker(ctx), database.UpsertBoundaryUsageStatsParams{
		ReplicaID:             replicaID,
		UniqueWorkspacesCount: workspaceCount, // cumulative, for UPDATE
		UniqueUsersCount:      userCount,      // cumulative, for UPDATE
		UniqueWorkspacesDelta: workspaceDelta, // delta, for INSERT
		UniqueUsersDelta:      userDelta,      // delta, for INSERT
		AllowedRequests:       allowed,
		DeniedRequests:        denied,
	})

	// Always reset cumulative counts to prevent unbounded memory growth (e.g.
	// if the DB is unreachable). Copy delta maps to preserve any Track() calls
	// that occurred during the DB operation above.
	t.mu.Lock()
	t.workspaces = make(map[uuid.UUID]struct{})
	t.users = make(map[uuid.UUID]struct{})
	for id := range t.workspacesDelta {
		t.workspaces[id] = struct{}{}
	}
	for id := range t.usersDelta {
		t.users[id] = struct{}{}
	}
	t.mu.Unlock()

	return err
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
