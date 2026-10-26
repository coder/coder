//go:build linux || darwin

package agentpreview

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/testutil"
)

func TestPreviewsRejectFIFOs(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe"), 0o600))
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe.html"), 0o600))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(root)
	require.NoError(t, err)
	response := previewRequest(t, api, http.MethodGet, "/"+id+"/pipe")
	require.Equal(t, http.StatusNotFound, response.Code)

	_, _, err = api.publish(filepath.Join(root, "pipe.html"))
	require.ErrorIs(t, err, errNotServed)
}

func TestPreviewsUnreadableFile(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root bypasses file mode permissions")
	}

	root := testutil.TempDirResolved(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locked.css"), []byte("locked"), 0o000))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(root)
	require.NoError(t, err)

	response := previewRequest(t, api, http.MethodGet, "/"+id+"/locked.css")
	require.Equal(t, http.StatusInternalServerError, response.Code)
}

func TestPreviewDirectoryOpensRejectFIFO(t *testing.T) {
	t.Parallel()

	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			t.Parallel()

			directory := testutil.TempDirResolved(t)
			fifo := filepath.Join(directory, "pipe")
			require.NoError(t, unix.Mkfifo(fifo, 0o600))
			parent, err := openPreviewRoot(directory)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, parent.Close()) })

			done := make(chan struct{})
			result := make(chan error, 1)
			// If an open wrongly blocks on the FIFO, opening the FIFO here
			// unblocks it, so a failing test does not leave the goroutine below
			// stuck in the syscall.
			t.Cleanup(func() {
				fd, err := unix.Open(fifo, unix.O_RDWR|unix.O_NONBLOCK, 0)
				require.NoError(t, err)
				defer func() { require.NoError(t, unix.Close(fd)) }()
				<-done
			})
			go func() {
				defer close(done)

				var root *os.Root
				var err error
				if nested {
					root, err = openPreviewSubroot(parent, "pipe")
				} else {
					root, err = openPreviewRoot(fifo)
				}
				if root != nil {
					_ = root.Close()
				}
				result <- err
			}()

			ctx := testutil.Context(t, testutil.WaitShort)
			require.Error(t, testutil.RequireReceive(ctx, t, result))
		})
	}
}
