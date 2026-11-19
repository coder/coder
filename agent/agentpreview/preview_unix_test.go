//go:build linux || darwin

package agentpreview_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/agent/agentpreview"
	"github.com/coder/coder/v2/testutil"
)

// Opening a FIFO blocks until a writer opens it, so previews must refuse
// FIFOs instead of opening them.
func TestPreviewRejectsFIFOs(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe"), 0o600))
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe.html"), 0o600))

	handler := agentpreview.NewAPI(testutil.Logger(t)).Routes()
	id := publish(t, handler, root).ID
	response := serve(t, handler, http.MethodGet, "/"+id+"/pipe")
	require.Equal(t, http.StatusNotFound, response.Code)

	response = publishResponse(t, handler, filepath.Join(root, "pipe.html"))
	require.Equal(t, http.StatusBadRequest, response.Code)
}
