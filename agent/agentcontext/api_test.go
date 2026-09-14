package agentcontext_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
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
