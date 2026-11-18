package agentgit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestPathStore_EvictsReadPathsBeforeWritten pins the per-chat cap:
// over the cap, the least recently used read-only paths go first, and
// written paths survive any number of reads.
func TestPathStore_EvictsReadPathsBeforeWritten(t *testing.T) {
	t.Parallel()

	ps := NewPathStore()
	chatID := uuid.New()
	ids := []uuid.UUID{chatID}

	ps.AddPaths(ids, []string{"/written"})
	// A read of a written path keeps it written.
	ps.AddReadPaths(ids, []string{"/written"})
	// A write of a read path makes it written.
	ps.AddReadPaths(ids, []string{"/upgraded"})
	ps.AddPaths(ids, []string{"/upgraded"})
	ps.AddReadPaths(ids, []string{"/read-old"})
	for i := range maxPathsPerChat {
		ps.AddReadPaths(ids, []string{fmt.Sprintf("/read-%04d", i)})
		if i == maxPathsPerChat/2 {
			// Re-reading refreshes recency.
			ps.AddReadPaths(ids, []string{"/read-old"})
		}
	}

	paths := ps.GetPaths(chatID)
	require.Len(t, paths, maxPathsPerChat)
	require.Contains(t, paths, "/written")
	require.Contains(t, paths, "/upgraded")
	require.Contains(t, paths, "/read-old")
	require.NotContains(t, paths, "/read-0000")
	require.NotContains(t, paths, "/read-0001")
	require.NotContains(t, paths, "/read-0002")
	require.Contains(t, paths, "/read-0003")

	// Written paths are evicted only when nothing read-only is left.
	for i := range maxPathsPerChat {
		ps.AddPaths(ids, []string{fmt.Sprintf("/write-%04d", i)})
	}
	paths = ps.GetPaths(chatID)
	require.Len(t, paths, maxPathsPerChat)
	require.NotContains(t, paths, "/read-old")
	require.NotContains(t, paths, "/written")
	require.Contains(t, paths, "/write-0000")
}
