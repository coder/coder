package files

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/lazy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Cache persists the files for template versions, and is used by dynamic
// parameters to deduplicate the files in memory.
//   - The user connects to the dynamic parameters websocket with a given template
//     version id.
//   - template version -> provisioner job -> file
//   - We persist those files
//
// Requirements:
//   - Multiple template versions can share a single "file"
//   - Files should be "ref counted" so that they're released when no one is using
//     them
//   - You should be able to fetch multiple different files in parallel, but you
//     should not fetch the same file multiple times in parallel.
type Cache struct {
	sync.Mutex
	data map[uuid.UUID]*lazy.Value[fs.FS]
}

// type CacheEntry struct {
// 	atomic.
// }

// Acquire
func (c *Cache) Acquire(fileID uuid.UUID) fs.FS {
	return c.fetch(fileID).Load()
}

// fetch handles grabbing the lock, creating a new lazy.Value if necessary,
// and returning it. The lock can be safely released because lazy.Value handles
// its own synchronization, so multiple concurrent reads for the same fileID
// will still only ever result in a single load being performed.
func (c *Cache) fetch(fileID uuid.UUID) *lazy.Value[fs.FS] {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	entry := c.data[fileID]
	if entry == nil {
		entry = lazy.New(func() fs.FS {
			time.Sleep(5 * time.Second)
			return NilFS{}
		})
		c.data[fileID] = entry
	}

	return entry
}

func NewFromStore(store database.Store) Cache {
	_ = func(ctx context.Context, fileID uuid.UUID) (fs.FS, error) {
		file, err := store.GetFileByID(ctx, fileID)
		if err != nil {
			return nil, xerrors.Errorf("failed to read file from database: %w", err)
		}

		reader := tar.NewReader(bytes.NewBuffer(file.Data))
		_, _ = io.ReadAll(reader)

		return NilFS{}, nil
	}

	return Cache{}
}

type NilFS struct{}

var _ fs.FS = NilFS{}

func (t NilFS) Open(_ string) (fs.File, error) {
	return nil, fmt.Errorf("oh no")
}
