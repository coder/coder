package agentcontext_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func newAPITestServer(t *testing.T, opts agentcontext.ManagerOptions) (*httptest.Server, *agentcontext.Manager) {
	t.Helper()
	m := newTestManager(t, opts)
	api := agentcontext.NewAPI(m)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)
	return srv, m
}

// doRequest issues an HTTP request bounded by testutil.WaitShort
// and returns the status code and response body. The response
// body is closed before doRequest returns.
func doRequest(t *testing.T, method, requrl string, body io.Reader) (int, []byte) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, method, requrl, body)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed below.
	require.NoError(t, err)
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, bodyBytes
}

func TestAPI_ListSourcesEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	status, body := doRequest(t, http.MethodGet, srv.URL+"/sources", nil)
	require.Equal(t, http.StatusOK, status)

	var got []agentcontext.SourceResponse
	require.NoError(t, json.Unmarshal(body, &got))
	require.Empty(t, got)
}

func TestAPI_AddAndListSource(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := testutil.TempDirResolved(t)

	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	body, _ := json.Marshal(agentcontext.SourceRequest{Path: src})
	status, addBody := doRequest(t, http.MethodPost, srv.URL+"/sources", bytes.NewReader(body))
	require.Equal(t, http.StatusCreated, status)

	var created agentcontext.SourceResponse
	require.NoError(t, json.Unmarshal(addBody, &created))
	require.Equal(t, src, created.Path)

	// List should show the new source.
	listStatus, listBody := doRequest(t, http.MethodGet, srv.URL+"/sources", nil)
	require.Equal(t, http.StatusOK, listStatus)
	var list []agentcontext.SourceResponse
	require.NoError(t, json.Unmarshal(listBody, &list))
	require.Len(t, list, 1)
	require.Equal(t, src, list[0].Path)
}

func TestAPI_AddSourceRejected(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	outside := t.TempDir()

	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd},
	})

	body, _ := json.Marshal(agentcontext.SourceRequest{Path: outside})
	status, _ := doRequest(t, http.MethodPost, srv.URL+"/sources", bytes.NewReader(body))
	require.Equal(t, http.StatusBadRequest, status)
}

func TestAPI_GetAndDeleteSource(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	src := testutil.TempDirResolved(t)

	srv, m := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir:   func() string { return wd },
		AllowedRoots: []string{wd, src},
	})

	_, err := m.AddSource(agentcontext.Source{Path: src})
	require.NoError(t, err)

	status, body := doRequest(t, http.MethodGet, srv.URL+"/sources/"+url.PathEscape(src), nil)
	require.Equal(t, http.StatusOK, status)

	var got agentcontext.SourceResponse
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, src, got.Path)

	delStatus, _ := doRequest(t, http.MethodDelete, srv.URL+"/sources/"+url.PathEscape(src), nil)
	require.Equal(t, http.StatusNoContent, delStatus)
	require.Empty(t, m.Sources())
}

func TestAPI_GetSourceNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	status, _ := doRequest(t, http.MethodGet, srv.URL+"/sources/"+url.PathEscape("/never-added"), nil)
	require.Equal(t, http.StatusNotFound, status)
}

func TestAPI_DeleteSourceNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	status, _ := doRequest(t, http.MethodDelete, srv.URL+"/sources/"+url.PathEscape("/never-added"), nil)
	require.Equal(t, http.StatusNotFound, status)
}

func TestAPI_Resync(t *testing.T) {
	t.Parallel()
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, "AGENTS.md"), "hello")

	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})

	status, body := doRequest(t, http.MethodPost, srv.URL+"/resync", nil)
	require.Equal(t, http.StatusOK, status)

	var snap agentcontext.SnapshotResponse
	require.NoError(t, json.Unmarshal(body, &snap))
	require.NotEmpty(t, snap.AggregateHash)
	require.Len(t, snap.Resources, 1)
	require.Equal(t, "instruction_file", snap.Resources[0].Kind)
	require.Equal(t, "ok", snap.Resources[0].Status)
}

func TestAPI_AddSourceMalformedBody(t *testing.T) {
	t.Parallel()
	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return t.TempDir() },
	})

	status, _ := doRequest(t, http.MethodPost, srv.URL+"/sources", bytes.NewReader([]byte("{not json")))
	require.Equal(t, http.StatusBadRequest, status)
}

func TestAPI_ResolveInstructions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	root := testutil.TempDirResolved(t)
	outside := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(root, "site", "AGENTS.md"), "site rules")
	mustWriteFile(t, filepath.Join(root, "site", "CLAUDE.md"), "claude rules")
	mustWriteFile(t, filepath.Join(root, "wrongcase", "agents.md"), "wrong case")
	mustWriteFile(t, filepath.Join(root, "plain", "README.md"), "no instructions")
	mustWriteFile(t, filepath.Join(root, "big", "AGENTS.md"), "this file is too large")
	mustWriteFile(t, filepath.Join(root, "binary", "AGENTS.md"), "\xff\xfe rules")
	mustWriteFile(t, filepath.Join(outside, "AGENTS.md"), "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "linked"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "AGENTS.md"), filepath.Join(root, "linked", "AGENTS.md")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dirnamed", "AGENTS.md"), 0o755))
	mustWriteFile(t, filepath.Join(root, "aliased", "docs", "rules.md"), "aliased rules")
	require.NoError(t, os.Symlink(filepath.Join("docs", "rules.md"), filepath.Join(root, "aliased", "AGENTS.md")))
	require.NoError(t, os.Symlink("AGENTS.md", filepath.Join(root, "aliased", "CLAUDE.md")))

	srv, _ := newAPITestServer(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return root },
		Resolver:   &agentcontext.Resolver{MaxResourceBytes: 16},
	})
	resolve := func(dirs ...string) (int, workspacesdk.ResolveContextInstructionsResponse) {
		t.Helper()
		body, err := json.Marshal(workspacesdk.ResolveContextInstructionsRequest{Directories: dirs})
		require.NoError(t, err)
		status, raw := doRequest(t, http.MethodPost, srv.URL+"/instructions", bytes.NewReader(body))
		var resp workspacesdk.ResolveContextInstructionsResponse
		if status == http.StatusOK {
			require.NoError(t, json.Unmarshal(raw, &resp))
		}
		return status, resp
	}

	status, resp := resolve(
		filepath.Join(root, "site"),
		filepath.Join(root, "plain"),
		filepath.Join(root, "missing"),
		filepath.Join(root, "wrongcase"),
		filepath.Join(root, "big"),
		filepath.Join(root, "binary"),
		filepath.Join(root, "linked"),
		filepath.Join(root, "dirnamed"),
		filepath.Join(root, "aliased"),
		filepath.Join(root, "site", "..", "site"),
	)
	require.Equal(t, http.StatusOK, status)
	bySource := make(map[string]workspacesdk.ContextInstructionFile, len(resp.Files))
	for _, file := range resp.Files {
		bySource[file.Source] = file
	}
	require.Len(t, bySource, 6, "plain, missing, wrong-case, and directory-named entries contribute nothing: %+v", resp.Files)
	require.Len(t, resp.Files, 8, "a directory listed twice is read twice")

	site := bySource[filepath.Join(root, "site", "AGENTS.md")]
	require.Equal(t, filepath.Join(root, "site"), site.Directory)
	require.Equal(t, "ok", site.Status)
	require.Equal(t, "site rules", site.Content)
	require.EqualValues(t, len("site rules"), site.SizeBytes)
	require.Len(t, site.ContentHash, 64, "sha256 hex")
	require.Equal(t, "claude rules", bySource[filepath.Join(root, "site", "CLAUDE.md")].Content)

	binary := bySource[filepath.Join(root, "binary", "AGENTS.md")]
	require.Equal(t, "ok", binary.Status)
	require.Equal(t, "\uFFFD rules", binary.Content, "invalid UTF-8 is replaced before the content is shipped")
	require.EqualValues(t, len(binary.Content), binary.SizeBytes, "the size counts the shipped content, not the file")

	big := bySource[filepath.Join(root, "big", "AGENTS.md")]
	require.Equal(t, "oversize", big.Status)
	require.Empty(t, big.Content)
	require.NotEmpty(t, big.Error)

	linked := bySource[filepath.Join(root, "linked", "AGENTS.md")]
	require.Equal(t, "invalid", linked.Status)
	require.Empty(t, linked.Content, "an escaping symlink target is never shipped")
	require.Contains(t, linked.Error, "escapes scan root")

	aliased := bySource[filepath.Join(root, "aliased", "AGENTS.md")]
	require.Equal(t, "ok", aliased.Status)
	require.Equal(t, "aliased rules", aliased.Content, "an in-tree symlink is read and reported under the link's path, where it applies")
	require.NotContains(t, bySource, filepath.Join(root, "aliased", "docs", "rules.md"))
	require.NotContains(t, bySource, filepath.Join(root, "aliased", "CLAUDE.md"), "a second name for the same file collapses onto the first")

	status, _ = resolve("relative/dir")
	require.Equal(t, http.StatusBadRequest, status)

	tooMany := make([]string, 0, workspacesdk.MaxContextInstructionDirectories+1)
	for range workspacesdk.MaxContextInstructionDirectories + 1 {
		tooMany = append(tooMany, filepath.Join(root, "site"))
	}
	status, _ = resolve(tooMany...)
	require.Equal(t, http.StatusBadRequest, status)

	status, resp = resolve()
	require.Equal(t, http.StatusOK, status)
	require.Empty(t, resp.Files)
}
