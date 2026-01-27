package boundaryusage

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Tracker tracks boundary usage for telemetry reporting.
//
// All stats accumulate in memory throughout a telemetry period and are only
// reset when a new period begins.
type Tracker struct {
	mu              sync.Mutex             //nolint:unused // Will be used when implemented.
	workspaces      map[uuid.UUID]struct{} //nolint:unused // Will be used when implemented.
	users           map[uuid.UUID]struct{} //nolint:unused // Will be used when implemented.
	allowedRequests int64                  //nolint:unused // Will be used when implemented.
	deniedRequests  int64                  //nolint:unused // Will be used when implemented.
}

// NewTracker creates a new boundary usage tracker.
func NewTracker() (*Tracker, error) {
	return nil, xerrors.New("not implemented")
}

// Track records boundary usage for a workspace.
func (*Tracker) Track(_, _ uuid.UUID, _, _ int64) error {
	return xerrors.New("not implemented")
}

// FlushToDB writes the accumulated stats to the database. All values are
// replaced in the database (they represent the current in-memory state). If the
// database row was deleted (new telemetry period), all in-memory stats are reset.
func (*Tracker) FlushToDB(_ context.Context, _ database.Store, _ uuid.UUID) error {
	return xerrors.New("not implemented")
}
