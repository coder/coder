package agentpreview

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestPreviewsFileIsolationAndLiveEdits(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	selected := filepath.Join(root, "page.html")
	require.NoError(t, os.WriteFile(selected, []byte("<h1>first</h1>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "private.html"), []byte("private"), 0o600))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(selected)
	require.NoError(t, err)

	response := previewRequest(t, api, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>first</h1>", response.Body.String())
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))

	for _, path := range []string{
		"/" + id + "/private.html",
		"/" + id + "/../private.html",
		"/" + id + "/%2e%2e/private.html",
		"/ffffffff/",
	} {
		response = previewRequest(t, api, http.MethodGet, path)
		require.Equal(t, http.StatusNotFound, response.Code, path)
	}

	// Saves that write a temporary file and rename it over the published
	// file must be served.
	temporary := filepath.Join(root, "save.html")
	require.NoError(t, os.WriteFile(temporary, []byte("<h1>second</h1>"), 0o600))
	require.NoError(t, os.Rename(temporary, selected))
	response = previewRequest(t, api, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>second</h1>", response.Body.String())

	response = previewRequest(t, api, http.MethodHead, "/"+id+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Body.String())

	response = previewRequest(t, api, http.MethodPost, "/"+id+"/")
	require.Equal(t, http.StatusMethodNotAllowed, response.Code)
}

func TestPreviewsDirectoryBoundaries(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	root := filepath.Join(parent, "public")
	require.NoError(t, os.Mkdir(root, 0o700))
	for name, body := range map[string]string{
		"index.html":          "index",
		"assets/style.css":    "body{}",
		"assets/fonts/a.css":  "font{}",
		"nested/index.html":   "nested",
		".hidden/token":       "hidden",
		"\u200c.hidden/token": "hidden",
		"empty/asset.txt":     "asset",
		"regular.txt":         "regular",
	} {
		name = filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
		require.NoError(t, os.WriteFile(name, []byte(body), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside"), 0o600))
	require.NoError(t, os.Link(filepath.Join(parent, "outside.txt"), filepath.Join(root, "linked.txt")))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(root)
	require.NoError(t, err)

	for _, tc := range []struct {
		path   string
		body   string
		status int
	}{
		{"/", "index", http.StatusOK},
		{"/assets/style.css", "body{}", http.StatusOK},
		{"/assets/fonts/a.css", "font{}", http.StatusOK},
		{"/regular.txt", "regular", http.StatusOK},
		{"/nested/", "nested", http.StatusOK},
		{"/empty/", "", http.StatusNotFound},
		{"/../outside.txt", "", http.StatusNotFound},
		{"/%2e%2e/outside.txt", "", http.StatusNotFound},
		{"/%2ehidden/token", "", http.StatusNotFound},
		// HFS+ ignores U+200C, so this name opens .hidden there.
		{"/%E2%80%8C.hidden/token", "", http.StatusNotFound},
		// Windows reads a backslash as a separator.
		{"/nested%5C..%5C.hidden%5Ctoken", "", http.StatusNotFound},
		{"/linked.txt", "", http.StatusNotFound},
	} {
		response := previewRequest(t, api, http.MethodGet, "/"+id+tc.path)
		require.Equal(t, tc.status, response.Code, tc.path)
		if tc.status == http.StatusOK {
			require.Equal(t, tc.body, response.Body.String(), tc.path)
		}
	}

	// The request log names the reason.
	for name, reason := range map[string]error{
		".hidden/token":     errNameNotServed,
		"assets//style.css": errNameNotServed,
		"linked.txt":        errNotServed,
		"regular.txt/x":     fs.ErrNotExist,
	} {
		_, _, err := api.openServed(id, name)
		require.ErrorIs(t, err, reason, name)
	}
}

func TestPreviewsLimitAndClose(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	index := filepath.Join(root, "index.html")
	require.NoError(t, os.WriteFile(index, []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.html"), []byte("other"), 0o600))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })

	id, canonical, err := api.publish(index)
	require.NoError(t, err)
	require.Regexp(t, "^[0-9a-f]{8}$", id)
	require.Equal(t, index, canonical)

	response := previewRequest(t, api, http.MethodGet, "/"+id+"/other.html")
	require.Equal(t, http.StatusNotFound, response.Code)

	directoryID, _, err := api.publish(root)
	require.NoError(t, err)
	require.NotEqual(t, id, directoryID)
	response = previewRequest(t, api, http.MethodGet, "/"+directoryID+"/other.html")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "other", response.Body.String())

	// maxPreviews rejects new paths but still allows republishing an existing
	// one.
	for i := range maxPreviews - 2 {
		name := filepath.Join(root, fmt.Sprintf("%d.html", i))
		require.NoError(t, os.WriteFile(name, []byte("page"), 0o600))
		_, _, err := api.publish(name)
		require.NoError(t, err)
	}
	_, _, err = api.publish(filepath.Join(root, "other.html"))
	require.ErrorContains(t, err, "at most")

	indexID, _, err := api.publish(index)
	require.NoError(t, err)
	require.Equal(t, id, indexID)

	require.NoError(t, api.Close())
	response = previewRequest(t, api, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusNotFound, response.Code)

	_, _, err = api.publish(index)
	require.ErrorIs(t, err, errPreviewsClosed)
}

func TestPreviewsInvalidPublications(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.htm"), []byte("page"), 0o600))

	for _, tc := range []struct {
		name    string
		message string
	}{
		{"page.html", "must be absolute"},
		{filepath.Join(root, "missing.html"), "does not exist or cannot be read"},
		{filepath.Join(root, "file.txt"), "requires an .html file"},
		{filepath.Join(root, "page.htm"), "requires an .html file"},
		{root, "index.html: preview path does not exist"},
	} {
		_, _, err := api.publish(tc.name)
		require.ErrorContains(t, err, tc.message, tc.name)
		// Errors reach the chat's AI model, so they must not leak OS error
		// details.
		require.NotContains(t, err.Error(), root, tc.name)
	}

	upper := filepath.Join(root, "PAGE.HTML")
	require.NoError(t, os.WriteFile(upper, []byte("page"), 0o600))
	_, _, err := api.publish(upper)
	require.NoError(t, err)
}

// IDs are random, so an ID from before an agent restart can't serve the path
// that gets an ID after it.
func TestPreviewsIDsAcrossRestarts(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	for _, name := range []string{"a.html", "b.html"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o600))
	}

	before := NewAPI(testutil.Logger(t))
	oldID, _, err := before.publish(filepath.Join(root, "a.html"))
	require.NoError(t, err)
	require.NoError(t, before.Close())

	after := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, after.Close()) })
	_, _, err = after.publish(filepath.Join(root, "b.html"))
	require.NoError(t, err)

	response := previewRequest(t, after, http.MethodGet, "/"+oldID+"/")
	require.Equal(t, http.StatusNotFound, response.Code)
}

// Republishing a rebuilt output directory serves the new tree.
func TestPreviewsRootReplacement(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	directory := filepath.Join(parent, "public")
	index := filepath.Join(directory, "index.html")
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(index, []byte("old tree"), 0o600))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(directory)
	require.NoError(t, err)
	replaced := api.previews[id].root

	require.NoError(t, os.Rename(directory, filepath.Join(parent, "previous")))
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(index, []byte("new tree"), 0o600))

	republished, _, err := api.publish(directory)
	require.NoError(t, err)
	require.Equal(t, id, republished)
	_, err = replaced.Stat(".")
	require.ErrorIs(t, err, os.ErrClosed)

	response := previewRequest(t, api, http.MethodGet, "/"+id+"/")
	require.Equal(t, "new tree", response.Body.String())
}

func TestPreviewsRepublishDuringRequests(t *testing.T) {
	t.Parallel()

	directory := testutil.TempDirResolved(t)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "assets", "app.js"), []byte("app"), 0o600))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	id, _, err := api.publish(directory)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				for _, path := range []string{"/" + id + "/", "/" + id + "/assets/app.js"} {
					response := previewRequest(t, api, http.MethodGet, path)
					assert.Equal(t, http.StatusOK, response.Code, path)
				}
			}
		})
	}
	for range 100 {
		_, _, err := api.publish(directory)
		require.NoError(t, err)
	}
	wg.Wait()
}

func TestPreviewsSymlinks(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	root := filepath.Join(parent, "public")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "assets", "style.css"), []byte("body{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside"), 0o600))
	symlink(t, "../outside.txt", filepath.Join(root, "escape.txt"))
	symlink(t, "assets", filepath.Join(root, "directory-alias"))

	api := NewAPI(testutil.Logger(t))
	t.Cleanup(func() { require.NoError(t, api.Close()) })

	// Publishing through a symlink publishes its target under the target's
	// preview ID.
	index := filepath.Join(root, "index.html")
	alias := filepath.Join(parent, "alias.html")
	symlink(t, index, alias)
	indexID, _, err := api.publish(index)
	require.NoError(t, err)
	aliasID, canonical, err := api.publish(alias)
	require.NoError(t, err)
	require.Equal(t, indexID, aliasID)
	require.Equal(t, index, canonical)

	id, _, err := api.publish(root)
	require.NoError(t, err)
	for _, path := range []string{"/escape.txt", "/directory-alias/style.css"} {
		response := previewRequest(t, api, http.MethodGet, "/"+id+path)
		require.Equal(t, http.StatusNotFound, response.Code, path)
		// The request log names the reason.
		_, _, err := api.openServed(id, path[1:])
		require.ErrorIs(t, err, errNotServed, path)
	}

	// Replacing a published file with a symlink must not serve another file
	// from its directory.
	selected := filepath.Join(parent, "page.html")
	require.NoError(t, os.WriteFile(selected, []byte("page"), 0o600))
	fileID, _, err := api.publish(selected)
	require.NoError(t, err)
	require.NoError(t, os.Remove(selected))
	symlink(t, "outside.txt", selected)
	response := previewRequest(t, api, http.MethodGet, "/"+fileID+"/")
	require.Equal(t, http.StatusNotFound, response.Code)

	linked := filepath.Join(parent, "linked")
	require.NoError(t, os.Mkdir(linked, 0o700))
	symlink(t, "../outside.txt", filepath.Join(linked, "index.html"))
	_, _, err = api.publish(linked)
	require.ErrorIs(t, err, errNotServed)
}

func symlink(t *testing.T, target, name string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	require.NoError(t, os.Symlink(target, name))
}

func previewRequest(t *testing.T, api *API, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, path, nil)
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, request)

	return recorder
}
