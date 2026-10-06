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

	logOutput, err := os.ReadFile(filepath.Join("testdata", "devcontainercli", "parse", "up-waitfor.log"))
	require.NoError(t, err)
	hookStartIndex := bytes.Index(logOutput, []byte(`"name":"Running postCreateCommand...","status":"running"`))
	require.Positive(t, hookStartIndex)
	hookLineEnd := hookStartIndex + bytes.IndexByte(logOutput[hookStartIndex:], '\n')

	var callbackCount, readyOffset int
	writer := &lifecycleReadyWriter{progressNames: lifecycleProgressNamesAfter("updateContentCommand")}
	for offset := range logOutput {
		writer.onReady = func() {
			callbackCount++
			readyOffset = offset
		}
		n, err := writer.Write(logOutput[offset : offset+1])
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	require.Equal(t, 1, callbackCount)
	require.Equal(t, hookLineEnd, readyOffset, "ready on the newline that ends the postCreateCommand start line")
}
