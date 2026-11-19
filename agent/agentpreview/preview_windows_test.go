//go:build windows

package agentpreview_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/coder/coder/v2/agent/agentpreview"
	"github.com/coder/coder/v2/testutil"
)

// Windows opens .hidden through its short name, such as HIDDEN~1, so short
// names must not get past the dotfile check.
func TestPreviewRejectsShortNames(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	hidden := filepath.Join(root, ".hidden")
	require.NoError(t, os.Mkdir(hidden, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(hidden, "token"), []byte("hidden"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))

	long, err := windows.UTF16PtrFromString(hidden)
	require.NoError(t, err)
	var buffer [windows.MAX_PATH]uint16
	n, err := windows.GetShortPathName(long, &buffer[0], windows.MAX_PATH)
	require.NoError(t, err)
	short := filepath.Base(windows.UTF16ToString(buffer[:n]))
	if short == ".hidden" {
		t.Skip("the volume does not create short names")
	}

	handler := agentpreview.NewAPI().Routes()
	id := string(publish(t, handler, root).ID)
	response := serve(t, handler, http.MethodGet, "/"+id+"/"+short+"/token")
	require.Equal(t, http.StatusNotFound, response.Code)
}
