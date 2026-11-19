package agentpreview_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentpreview"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPreviewFile(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	selected := filepath.Join(root, "page.html")
	require.NoError(t, os.WriteFile(selected, []byte("<h1>first</h1>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "private.html"), []byte("private"), 0o600))

	handler := agentpreview.NewAPI().Routes()
	published := publish(t, handler, selected)
	require.Equal(t, selected, published.Path)
	require.True(t, published.File)

	response := serve(t, handler, http.MethodGet, "/"+string(published.ID)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>first</h1>", response.Body.String())
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))

	for _, path := range []string{"/" + string(published.ID) + "/private.html", "/ffffffff/"} {
		response = serve(t, handler, http.MethodGet, path)
		require.Equal(t, http.StatusNotFound, response.Code, path)
	}

	temporary := filepath.Join(root, "save.html")
	require.NoError(t, os.WriteFile(temporary, []byte("<h1>second</h1>"), 0o600))
	require.NoError(t, os.Rename(temporary, selected))
	response = serve(t, handler, http.MethodGet, "/"+string(published.ID)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "<h1>second</h1>", response.Body.String())

	response = serve(t, handler, http.MethodHead, "/"+string(published.ID)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Body.String())

	require.Equal(t, published.ID, publish(t, handler, selected).ID)
}

func TestPreviewDirectory(t *testing.T) {
	t.Parallel()

	parent := testutil.TempDirResolved(t)
	root := filepath.Join(parent, "public")
	for name, body := range map[string]string{
		"index.html":         "index",
		"assets/style.css":   "body{}",
		"assets/fonts/a.css": "font{}",
		"nested/index.html":  "nested",
		"empty/asset.txt":    "asset",
		"vendors~main.js":    "bundle",
		".hidden/token":      "hidden",
	} {
		name = filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
		require.NoError(t, os.WriteFile(name, []byte(body), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside"), 0o600))

	handler := agentpreview.NewAPI().Routes()
	published := publish(t, handler, root)
	require.False(t, published.File)
	id := string(published.ID)

	// Only Windows reads a tilde as part of a short name such as HIDDEN~1.
	tildeStatus := http.StatusOK
	if runtime.GOOS == "windows" {
		tildeStatus = http.StatusNotFound
	}
	for _, tc := range []struct {
		path   string
		body   string
		status int
	}{
		{"/", "index", http.StatusOK},
		{"/assets/style.css", "body{}", http.StatusOK},
		{"/assets/fonts/a.css", "font{}", http.StatusOK},
		{"/nested/", "nested", http.StatusOK},
		{"/vendors~main.js", "bundle", tildeStatus},
		{"/empty/", "", http.StatusNotFound},
		{"/../outside.txt", "", http.StatusNotFound},
		{"/%2ehidden/token", "", http.StatusNotFound},
		// Windows reads a backslash as a separator.
		{"/nested%5C..%5C.hidden%5Ctoken", "", http.StatusNotFound},
	} {
		response := serve(t, handler, http.MethodGet, "/"+id+tc.path)
		require.Equal(t, tc.status, response.Code, tc.path)
		if tc.status == http.StatusOK {
			require.Equal(t, tc.body, response.Body.String(), tc.path)
		}
	}
}

func TestPreviewRebuiltDirectory(t *testing.T) {
	t.Parallel()

	root := filepath.Join(testutil.TempDirResolved(t), "dist")
	require.NoError(t, os.Mkdir(root, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("old build"), 0o600))

	handler := agentpreview.NewAPI().Routes()
	id := string(publish(t, handler, root).ID)

	require.NoError(t, os.RemoveAll(root))
	response := serve(t, handler, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusNotFound, response.Code)

	require.NoError(t, os.Mkdir(root, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("new build"), 0o600))
	response = serve(t, handler, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "new build", response.Body.String())
}

func TestPreviewSymlinks(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}

	parent := testutil.TempDirResolved(t)
	root := filepath.Join(parent, "public")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"), []byte("secret"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside"), 0o600))
	require.NoError(t, os.Symlink("../outside.txt", filepath.Join(root, "escape.txt")))
	require.NoError(t, os.Symlink("index.html", filepath.Join(root, "alias.html")))
	require.NoError(t, os.Symlink(".git", filepath.Join(root, "repo")))

	handler := agentpreview.NewAPI().Routes()
	id := string(publish(t, handler, root).ID)
	for _, path := range []string{"/escape.txt", "/alias.html", "/repo/config"} {
		response := serve(t, handler, http.MethodGet, "/"+id+path)
		require.Equal(t, http.StatusNotFound, response.Code, path)
	}
}

func TestPreviewInvalidPublications(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.htm"), []byte("page"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "PAGE.HTML"), []byte("page"), 0o600))

	handler := agentpreview.NewAPI().Routes()
	for _, name := range []string{
		"page.html",
		filepath.Join(root, "missing.html"),
		filepath.Join(root, "file.txt"),
		filepath.Join(root, "page.htm"),
		root,
	} {
		response := publishResponse(t, handler, name)
		require.Equal(t, http.StatusBadRequest, response.Code, name)
	}

	publish(t, handler, filepath.Join(root, "PAGE.HTML"))
}

func TestPreviewIDsAcrossRestarts(t *testing.T) {
	t.Parallel()

	page := filepath.Join(testutil.TempDirResolved(t), "page.html")
	require.NoError(t, os.WriteFile(page, []byte("page"), 0o600))

	before := agentpreview.NewAPI().Routes()
	id := publish(t, before, page).ID

	after := agentpreview.NewAPI().Routes()
	response := serve(t, after, http.MethodGet, "/"+string(id)+"/")
	require.Equal(t, http.StatusNotFound, response.Code)

	require.Equal(t, id, publish(t, after, page).ID)
	response = serve(t, after, http.MethodGet, "/"+string(id)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "page", response.Body.String())
}

func TestPreviewIDCollision(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	first, second := collidingPaths(root)
	for _, name := range []string{first, second} {
		require.NoError(t, os.WriteFile(name, []byte(name), 0o600))
	}

	handler := agentpreview.NewAPI().Routes()
	id := publish(t, handler, first).ID
	require.Equal(t, http.StatusConflict, publishResponse(t, handler, second).Code)

	response := serve(t, handler, http.MethodGet, "/"+string(id)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, first, response.Body.String())
}

// collidingPaths returns two .html files in dir whose preview IDs are equal.
func collidingPaths(dir string) (first, second string) {
	seen := make(map[appurl.PreviewID]string)
	for i := 0; ; i++ {
		name := filepath.Join(dir, strconv.Itoa(i)+".html")
		id := appurl.NewPreviewID(name)
		if other, ok := seen[id]; ok {
			return other, name
		}
		seen[id] = name
	}
}

func publish(t *testing.T, handler http.Handler, path string) workspacesdk.OpenPreviewResponse {
	t.Helper()

	response := publishResponse(t, handler, path)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var published workspacesdk.OpenPreviewResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&published))
	return published
}

func publishResponse(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(workspacesdk.OpenPreviewRequest{Path: path})
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), http.MethodPost, "/", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func serve(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
