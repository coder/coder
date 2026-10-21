package agentfiles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func requestFindFiles(t *testing.T, handler http.Handler, query url.Values) (int, workspacesdk.FindFilesResponse) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/find-files?"+query.Encode(), nil)
	handler.ServeHTTP(w, r)

	var resp workspacesdk.FindFilesResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	}
	return w.Code, resp
}

func TestHandleFindFiles(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	rootA := filepath.Join(home, "projA")
	rootB := filepath.Join(home, "projB")
	for _, p := range []string{
		filepath.Join(rootA, "cmd", "ServerMain.go"),
		filepath.Join(rootA, "README.md"),
		filepath.Join(rootB, "servermain_b.go"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, nil, 0o600))
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil,
		agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}),
		agentfiles.WithWorkingDir(func() string { return rootA }),
	)
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	handler := api.Routes()

	// Index both roots so the engine holds results for rootB.
	code, resp := requestFindFiles(t, handler, url.Values{"root": {rootA, "~/projB"}, "q": {"servermain"}})
	require.Equal(t, http.StatusOK, code)
	require.ElementsMatch(t, []workspacesdk.FindFilesResult{
		{AbsolutePath: filepath.Join(rootA, "cmd", "ServerMain.go"), Path: "cmd/ServerMain.go", Root: rootA},
		{AbsolutePath: filepath.Join(rootB, "servermain_b.go"), Path: "servermain_b.go", Root: rootB},
	}, withoutScores(resp.Results))

	t.Run("FiltersToRequestedRoot", func(t *testing.T) {
		t.Parallel()
		// An empty root is the default directory, rootA.
		code, resp := requestFindFiles(t, handler, url.Values{"root": {""}, "q": {"servermain"}})
		require.Equal(t, http.StatusOK, code)
		require.Len(t, resp.Results, 1)
		require.Equal(t, rootA, resp.Results[0].Root)
		require.Equal(t, "cmd/ServerMain.go", resp.Results[0].Path)
		require.Positive(t, resp.Results[0].Score)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		code, resp := requestFindFiles(t, handler, url.Values{"root": {rootA, rootB}, "q": {"servermain"}, "limit": {"1"}})
		require.Equal(t, http.StatusOK, code)
		require.Len(t, resp.Results, 1)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()
		for name, query := range map[string]url.Values{
			"MissingRoot":    {"q": {"main"}},
			"MissingQuery":   {"root": {rootA}},
			"EmptyQuery":     {"root": {rootA}, "q": {""}},
			"RootNotExist":   {"root": {filepath.Join(home, "nope")}, "q": {"main"}},
			"RootIsFile":     {"root": {filepath.Join(rootA, "README.md")}, "q": {"main"}},
			"LimitTooLarge":  {"root": {rootA}, "q": {"main"}, "limit": {"201"}},
			"UnknownParam":   {"root": {rootA}, "q": {"main"}, "bogus": {"1"}},
			"LimitNotNumber": {"root": {rootA}, "q": {"main"}, "limit": {"x"}},
		} {
			code, _ := requestFindFiles(t, handler, query)
			require.Equal(t, http.StatusBadRequest, code, name)
		}
	})
}

func withoutScores(results []workspacesdk.FindFilesResult) []workspacesdk.FindFilesResult {
	out := make([]workspacesdk.FindFilesResult, len(results))
	for i, r := range results {
		r.Score = 0
		out[i] = r
	}
	return out
}
