package agentcontainers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLifecycleReadyWriter_SplitWrites(t *testing.T) {
	t.Parallel()

	log, err := os.ReadFile(filepath.Join("testdata", "devcontainercli", "parse", "up-waitfor.log"))
	require.NoError(t, err)
	start := bytes.Index(log, []byte(`"name":"Running postCreateCommand...","status":"running"`))
	require.Positive(t, start)
	lineEnd := start + bytes.IndexByte(log[start:], '\n')

	var calls, readyAt int
	w := &lifecycleReadyWriter{names: lifecycleHooksAfter("updateContentCommand")}
	for i := range log {
		w.onReady = func() {
			calls++
			readyAt = i
		}
		n, err := w.Write(log[i : i+1])
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	require.Equal(t, 1, calls)
	require.Equal(t, lineEnd, readyAt, "ready on the newline that ends the postCreateCommand start line")
}
