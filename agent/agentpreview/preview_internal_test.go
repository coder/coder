//go:build linux || darwin

package agentpreview

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/testutil"
)

func TestPreviewsFileIsolationAndLiveEdits(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	selected := filepath.Join(root, "page.html")
	require.NoError(t, os.WriteFile(selected, []byte("<h1>first</h1>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "private.html"), []byte("private"), 0o600))

	p := NewAPI()
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	id, _, err := p.open(selected)
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{16}$`, id)

	response := previewRequest(t, p, http.MethodGet, "/"+id+"/", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>first</h1>", response.Body.String())
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))

	response = previewRequest(t, p, http.MethodGet, "/"+id, nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>first</h1>", response.Body.String())

	for _, path := range []string{
		"/" + id + "/page.html",
		"/" + id + "/private.html",
		"/" + id + "/index.html",
		"/" + id + "/../private.html",
		"/" + id + "/%2e%2e/private.html",
		"/0123456789abcdef/",
		"/",
	} {
		response = previewRequest(t, p, http.MethodGet, path, nil)
		require.Equal(t, http.StatusNotFound, response.Code, path)
	}

	// Atomic saves must be visible, including with stale HTTP validators.
	temporary := filepath.Join(root, "save.html")
	require.NoError(t, os.WriteFile(temporary, []byte("<h1>second</h1>"), 0o600))
	require.NoError(t, os.Rename(temporary, selected))
	response = previewRequest(t, p, http.MethodGet, "/"+id+"/", http.Header{
		"If-Modified-Since": {time.Now().Add(time.Hour).Format(http.TimeFormat)},
		"If-None-Match":     {"*"},
	})
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>second</h1>", response.Body.String())

	response = previewRequest(t, p, http.MethodGet, "/"+id+"/", http.Header{
		"Range": {"bytes=99999-"},
	})
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))

	response = previewRequest(t, p, http.MethodPost, "/"+id+"/", nil)
	require.Equal(t, http.StatusMethodNotAllowed, response.Code)

	// A replacement symlink must not publish another file in the parent.
	require.NoError(t, os.Remove(selected))
	require.NoError(t, os.Symlink("private.html", selected))
	response = previewRequest(t, p, http.MethodGet, "/"+id+"/", nil)
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestPreviewsDirectoryBoundaries(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	root := filepath.Join(parent, "public")
	require.NoError(t, os.Mkdir(root, 0o700))
	for name, body := range map[string]string{
		"index.html":        "index",
		"assets/style.css":  "body{}",
		"nested/index.html": "nested",
		".hidden/token":     "hidden",
		"empty/asset.txt":   "asset",
		"regular.txt":       "regular",
	} {
		name = filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
		require.NoError(t, os.WriteFile(name, []byte(body), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside"), 0o600))
	for name, target := range map[string]string{
		"escape.txt":        "../outside.txt",
		"hidden-alias":      ".hidden",
		"token.txt":         ".hidden/token",
		"regular-alias.txt": "regular.txt",
		"directory-alias":   "assets",
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(root, name)))
	}
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe"), 0o600))
	// Another link to a hard-linked file may sit outside the preview.
	require.NoError(t, os.Link(filepath.Join(parent, "outside.txt"), filepath.Join(root, "linked.txt")))

	p := NewAPI()
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	id, _, err := p.open(root)
	require.NoError(t, err)

	for _, tc := range []struct {
		path   string
		body   string
		status int
	}{
		{"/", "index", http.StatusOK},
		{"/assets/style.css", "body{}", http.StatusOK},
		{"/regular.txt", "regular", http.StatusOK},
		{"/nested/", "nested", http.StatusOK},
		{"/empty/", "", http.StatusNotFound},
		{"/empty", "", http.StatusNotFound},
		{"/../outside.txt", "", http.StatusNotFound},
		{"/%2e%2e/outside.txt", "", http.StatusNotFound},
		{"/assets/%2e%2e/%2e%2e/outside.txt", "", http.StatusNotFound},
		{"/%2ehidden/token", "", http.StatusNotFound},
		{"/hidden-alias/token", "", http.StatusNotFound},
		{"/token.txt", "", http.StatusNotFound},
		{"/regular-alias.txt", "", http.StatusNotFound},
		{"/directory-alias/style.css", "", http.StatusNotFound},
		{"/escape.txt", "", http.StatusNotFound},
		{"/pipe", "", http.StatusNotFound},
		{"/linked.txt", "", http.StatusNotFound},
		{"/assets%5c..%5coutside.txt", "", http.StatusNotFound},
	} {
		response := previewRequest(t, p, http.MethodGet, "/"+id+tc.path, nil)
		require.Equal(t, tc.status, response.Code, tc.path)
		if tc.status == http.StatusOK {
			require.Equal(t, tc.body, response.Body.String(), tc.path)
		}
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, "assets/style.css"), []byte("edited"), 0o600))
	response := previewRequest(t, p, http.MethodGet, "/"+id+"/assets/style.css", nil)
	require.Equal(t, "edited", response.Body.String())
}

func TestPreviewsReuseLimitAndClose(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	index := filepath.Join(root, "index.html")
	require.NoError(t, os.WriteFile(index, []byte("index"), 0o600))

	p := NewAPI()
	t.Cleanup(func() { require.NoError(t, p.Close()) })

	first, _, err := p.open(index)
	require.NoError(t, err)

	// Aliases resolve to the canonical path and reuse its ID.
	alias := filepath.Join(root, "alias.html")
	require.NoError(t, os.Symlink("index.html", alias))
	id, canonical, err := p.open(alias)
	require.NoError(t, err)
	require.Equal(t, first, id)
	require.Equal(t, index, canonical)

	directoryID, _, err := p.open(root)
	require.NoError(t, err)
	require.NotEqual(t, first, directoryID)

	// Republishing after a daemon restart keeps the preview URL.
	restarted := NewAPI()
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	id, _, err = restarted.open(index)
	require.NoError(t, err)
	require.Equal(t, first, id)

	// The limit rejects new previews but still allows republishing.
	for i := range maxPreviews - 2 {
		file := filepath.Join(root, fmt.Sprintf("page-%d.html", i))
		require.NoError(t, os.WriteFile(file, []byte("page"), 0o600))
		_, _, err := p.open(file)
		require.NoError(t, err)
	}
	file := filepath.Join(root, "overflow.html")
	require.NoError(t, os.WriteFile(file, []byte("overflow"), 0o600))
	_, _, err = p.open(file)
	require.ErrorContains(t, err, "at most")

	id, _, err = p.open(index)
	require.NoError(t, err)
	require.Equal(t, first, id)

	// Close races with republishing; opens either succeed or see Close.
	const openers = 8
	var closing sync.WaitGroup
	closeErrors := make(chan error, 1+openers)
	closing.Go(func() { closeErrors <- p.Close() })
	for range openers {
		closing.Go(func() {
			_, _, err := p.open(index)
			if err != nil && !errors.Is(err, errPreviewsClosed) {
				closeErrors <- err
			}
		})
	}
	closing.Wait()
	close(closeErrors)
	for err := range closeErrors {
		require.NoError(t, err)
	}

	response := previewRequest(t, p, http.MethodGet, "/"+first+"/", nil)
	require.Equal(t, http.StatusNotFound, response.Code)

	_, _, err = p.open(index)
	require.ErrorIs(t, err, errPreviewsClosed)
}

func TestPreviewsInvalidPublications(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	p := NewAPI()
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	for _, name := range []string{"file.txt", ".hidden.html"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("data"), 0o600))
	}
	require.NoError(t, unix.Mkfifo(filepath.Join(root, "pipe.html"), 0o600))

	for _, name := range []string{
		"relative.html",
		filepath.Join(root, "missing.html"),
		filepath.Join(root, "file.txt"),
		filepath.Join(root, ".hidden.html"),
		filepath.Join(root, "pipe.html"),
		root,
	} {
		_, _, err := p.open(name)
		require.Error(t, err, name)
		// Errors reach the model, so they must not leak OS error details.
		require.NotContains(t, err.Error(), root, name)
	}

	require.NoError(t, os.Symlink("file.txt", filepath.Join(root, "index.html")))
	_, _, err := p.open(root)
	require.Error(t, err)
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
			// Unblock an unsafe open before reporting a failing negative control,
			// so a detected regression does not strand a syscall goroutine.
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
			select {
			case err := <-result:
				require.Error(t, err)
			case <-ctx.Done():
				t.Fatal("directory open blocked on a FIFO")
			}
		})
	}
}

// Republishing a rebuilt output directory must serve the new tree, not the
// replaced one the preview pinned.
func TestPreviewsRootReplacement(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	directory := filepath.Join(parent, "public")
	index := filepath.Join(directory, "index.html")
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(index, []byte("old tree"), 0o600))

	p := NewAPI()
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	id, _, err := p.open(directory)
	require.NoError(t, err)

	require.NoError(t, os.Rename(directory, filepath.Join(parent, "previous")))
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(index, []byte("new tree"), 0o600))

	refreshed, _, err := p.open(directory)
	require.NoError(t, err)
	require.Equal(t, id, refreshed)

	response := previewRequest(t, p, http.MethodGet, "/"+id+"/", nil)
	require.Equal(t, "new tree", response.Body.String())
}

func previewRequest(t *testing.T, p *API, method, path string, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, path, nil)
	for name, values := range headers {
		request.Header[name] = values
	}
	recorder := httptest.NewRecorder()
	p.ServeHTTP(recorder, request)

	return recorder
}
