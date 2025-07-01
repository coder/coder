package files

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// LeakCache prevents entries from even being released to enable testing certain
// behaviors.
type LeakCache struct {
	*Cache
}

func (c *LeakCache) Acquire(ctx context.Context, db database.Store, fileID uuid.UUID) (*CloseFS, error) {
	// We need to call prepare first to both 1. leak a reference and 2. prevent
	// the behavior of immediately closing on an error (as implemented in Acquire)
	// from freeing the file.
	c.prepare(db, fileID)
	return c.Cache.Acquire(ctx, db, fileID)
}
