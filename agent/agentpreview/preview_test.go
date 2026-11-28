package agentpreview_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentpreview"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// plainNotFound is the body of a 404 for a path that a published preview does
// not serve.
const plainNotFound = "404 page not found\n"

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
	writeFiles(t, root, map[string]string{
		"index.html":         "index",
		"assets/style.css":   "body{}",
		"assets/fonts/a.css": "font{}",
		"nested/index.html":  "nested",
		"empty/asset.txt":    "asset",
		"vendors~main.js":    "bundle",
		".hidden/token":      "hidden",
	})
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
		body := tc.body
		if tc.status == http.StatusNotFound {
			body = plainNotFound
		}
		require.Equal(t, body, response.Body.String(), tc.path)
	}
}

func TestPreviewDirectoryRedirect(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"index.html":      "index",
		"docs/index.html": "docs",
		"a/b/index.html":  "b",
	}
	cases := []struct {
		path     string
		location string
		body     string
	}{
		{"/docs?x=1", "docs/?x=1", "docs"},
		{"/a/b", "b/", "b"},
	}
	// Windows refuses a colon in a file name.
	if runtime.GOOS != "windows" {
		files["a:b/index.html"] = "ab"
		cases = append(cases, struct{ path, location, body string }{"/a:b", "./a:b/", "ab"})
	}
	root := testutil.TempDirResolved(t)
	writeFiles(t, root, files)

	handler := agentpreview.NewAPI().Routes()
	id := string(publish(t, handler, root).ID)
	for _, tc := range cases {
		requested := "/" + id + tc.path
		response := serve(t, handler, http.MethodGet, requested)
		require.Equal(t, http.StatusMovedPermanently, response.Code, tc.path)
		require.Equal(t, tc.location, response.Header().Get("Location"), tc.path)

		base, err := url.Parse(requested)
		require.NoError(t, err)
		location, err := url.Parse(tc.location)
		require.NoError(t, err)
		response = serve(t, handler, http.MethodGet, base.ResolveReference(location).String())
		require.Equal(t, http.StatusOK, response.Code, tc.path)
		require.Equal(t, tc.body, response.Body.String(), tc.path)
	}

	// A request that already ends in a slash, or names the root, gets a 404
	// when its index.html is a directory. A redirect would point one level
	// deeper, at a path that does not exist.
	require.NoError(t, os.Remove(filepath.Join(root, "index.html")))
	writeFiles(t, root, map[string]string{"index.html/x": "x", "trap/index.html/x": "x"})
	for _, path := range []string{"/", "/trap/"} {
		response := serve(t, handler, http.MethodGet, "/"+id+path)
		require.Equal(t, http.StatusNotFound, response.Code, path)
		require.Empty(t, response.Header().Get("Location"), path)
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

	// The published directory itself replaced by a symlink.
	home := filepath.Join(parent, "home")
	require.NoError(t, os.Mkdir(home, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "index.html"), []byte("home"), 0o600))
	require.NoError(t, os.Rename(root, filepath.Join(parent, "old")))
	require.NoError(t, os.Symlink(home, root))
	response := serve(t, handler, http.MethodGet, "/"+id+"/")
	require.Equal(t, http.StatusNotFound, response.Code)
}

// A path under a symlinked directory publishes and serves from its real
// location, which later changes to the symlink do not move.
func TestPreviewSymlinkedParent(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}

	parent := testutil.TempDirResolved(t)
	actual, other, link := filepath.Join(parent, "actual"), filepath.Join(parent, "other"), filepath.Join(parent, "link")
	writeFiles(t, actual, map[string]string{"page.html": "page", "site/index.html": "site"})
	writeFiles(t, other, map[string]string{"page.html": "other page", "site/index.html": "other site"})
	require.NoError(t, os.Symlink(actual, link))

	handler := agentpreview.NewAPI().Routes()
	page := publish(t, handler, filepath.Join(link, "page.html"))
	require.Equal(t, filepath.Join(actual, "page.html"), page.Path)
	site := publish(t, handler, filepath.Join(link, "site"))
	require.Equal(t, filepath.Join(actual, "site"), site.Path)

	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(other, link))
	for id, body := range map[appurl.PreviewID]string{page.ID: "page", site.ID: "site"} {
		response := serve(t, handler, http.MethodGet, "/"+string(id)+"/")
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, body, response.Body.String())
	}

	for _, name := range []string{"page.html", "site"} {
		require.NoError(t, os.Rename(filepath.Join(actual, name), filepath.Join(actual, "old-"+name)))
		require.NoError(t, os.Symlink(filepath.Join(other, name), filepath.Join(actual, name)))
	}
	for _, id := range []appurl.PreviewID{page.ID, site.ID} {
		response := serve(t, handler, http.MethodGet, "/"+string(id)+"/")
		require.Equal(t, http.StatusNotFound, response.Code)
	}
}

// Swapping a path element for a symlink while requests run must never serve
// the symlink's target.
func TestPreviewSwappedElement(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}

	root := testutil.TempDirResolved(t)
	writeFiles(t, root, map[string]string{
		"index.html":    "index",
		"assets/config": "public",
		".git/config":   "secret",
	})
	assets, spare, link := filepath.Join(root, "assets"), filepath.Join(root, ".assets"), filepath.Join(root, ".link")
	require.NoError(t, os.Symlink(".git", link))

	handler := agentpreview.NewAPI().Routes()
	id := string(publish(t, handler, root).ID)

	stop := make(chan struct{})
	swapped := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				swapped <- nil
				return
			default:
			}
			for _, move := range [][2]string{{assets, spare}, {link, assets}, {assets, link}, {spare, assets}} {
				if err := os.Rename(move[0], move[1]); err != nil {
					swapped <- err
					return
				}
			}
		}
	}()
	var leaks int
	for range 10000 {
		if serve(t, handler, http.MethodGet, "/"+id+"/assets/config").Body.String() == "secret" {
			leaks++
		}
	}
	close(stop)
	require.NoError(t, <-swapped)
	require.Zero(t, leaks)
}

func TestPreviewInvalidPublications(t *testing.T) {
	t.Parallel()

	root := testutil.TempDirResolved(t)
	writeFiles(t, root, map[string]string{
		"file.txt":                    "data",
		"page.htm":                    "page",
		"PAGE.HTML":                   "page",
		"nested/index.html/page.html": "page",
		"site/index.html":             "site",
	})
	missing := filepath.Join(root, "missing.html")
	_, missingErr := os.Stat(missing)
	require.Error(t, missingErr)
	nested := filepath.Join(root, "nested")

	type refusal struct {
		path   string
		detail string
	}
	tests := []refusal{
		{"page.html", `preview path "page.html" must be absolute`},
		{missing, missingErr.Error()},
		{filepath.Join(root, "file.txt"), "preview requires an .html file or a directory with index.html"},
		{filepath.Join(root, "page.htm"), "preview requires an .html file or a directory with index.html"},
		{root, "directory " + root + " has no index.html; a directory preview needs index.html at its root"},
		{nested, "index.html in " + nested + " is a directory, not a file"},
	}
	// Creating symlinks requires admin privileges on Windows runners.
	if runtime.GOOS != "windows" {
		linkedIndex, linkedSite := filepath.Join(root, "linked"), filepath.Join(root, "alias")
		require.NoError(t, os.Mkdir(linkedIndex, 0o700))
		require.NoError(t, os.Symlink(filepath.Join(root, "site", "index.html"), filepath.Join(linkedIndex, "index.html")))
		require.NoError(t, os.Symlink(filepath.Join(root, "site"), linkedSite))
		tests = append(tests,
			refusal{linkedIndex, "index.html in " + linkedIndex + " is a symlink; previews do not follow symlinks"},
			refusal{linkedSite, "directory " + linkedSite + " is a symlink; previews do not follow symlinks"},
		)
	}

	handler := agentpreview.NewAPI().Routes()
	for _, tc := range tests {
		status, detail := publishError(t, handler, tc.path)
		require.Equal(t, http.StatusBadRequest, status, tc.path)
		require.Equal(t, tc.detail, detail, tc.path)
	}

	publish(t, handler, filepath.Join(root, "PAGE.HTML"))
}

func TestPreviewRepublishAsDirectory(t *testing.T) {
	t.Parallel()

	name := filepath.Join(testutil.TempDirResolved(t), "report.html")
	require.NoError(t, os.WriteFile(name, []byte("file"), 0o600))

	handler := agentpreview.NewAPI().Routes()
	published := publish(t, handler, name)
	require.True(t, published.File)

	require.NoError(t, os.Remove(name))
	require.NoError(t, os.Mkdir(name, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(name, "index.html"), []byte("directory"), 0o600))
	republished := publish(t, handler, name)
	require.Equal(t, published.ID, republished.ID)
	require.False(t, republished.File)

	response := serve(t, handler, http.MethodGet, "/"+string(published.ID)+"/")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "directory", response.Body.String())
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
	require.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	require.Contains(t, response.Body.String(), "This preview is no longer available because the workspace agent restarted. Ask the agent to open it again.")

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
	status, detail := publishError(t, handler, second)
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "preview ID "+string(id)+" is already used by "+first+"; rename or move one of the two paths and publish again", detail)

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

// publishError publishes path, expecting a refusal, and returns the status and
// the error detail.
func publishError(t *testing.T, handler http.Handler, path string) (int, string) {
	t.Helper()

	response := publishResponse(t, handler, path)
	var body codersdk.Response
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	return response.Code, body.Detail
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

// writeFiles writes each slash-separated name in files below root, creating
// its parent directories.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, body := range files {
		name = filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
		require.NoError(t, os.WriteFile(name, []byte(body), 0o600))
	}
}

func serve(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), method, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
