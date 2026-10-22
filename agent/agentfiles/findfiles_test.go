package agentfiles_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
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

func TestHandleFindFiles_RootIdentity(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	home := t.TempDir()
	rootA := filepath.Join(home, "projA")
	rootB := filepath.Join(home, "projB")
	require.NoError(t, os.MkdirAll(filepath.Join(rootA, "deep", "deeper"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "deep", "deeper", "x_widget_extra_long_name.go"), nil, 0o600))
	// rootB has many better-scoring matches that would crowd out
	// rootA's single match if every indexed root were searched.
	for i := range 40 {
		p := filepath.Join(rootB, fmt.Sprintf("d%d", i), "widget.go")
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, nil, 0o600))
	}
	link := filepath.Join(home, "link")
	require.NoError(t, os.Symlink(rootA, link))

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil,
		agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}),
		// An unclean configured directory must match the engine's
		// cleaned root.
		agentfiles.WithWorkingDir(func() string { return rootA + "/" }),
	)
	t.Cleanup(func() { require.NoError(t, api.Close()) })
	handler := api.Routes()

	code, resp := requestFindFiles(t, handler, url.Values{"root": {rootB}, "q": {"widget"}})
	require.Equal(t, http.StatusOK, code)
	require.NotEmpty(t, resp.Results)

	code, resp = requestFindFiles(t, handler, url.Values{"root": {""}, "q": {"widget"}, "limit": {"1"}})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, []workspacesdk.FindFilesResult{{
		AbsolutePath: filepath.Join(rootA, "deep", "deeper", "x_widget_extra_long_name.go"),
		Path:         "deep/deeper/x_widget_extra_long_name.go",
		Root:         rootA,
	}}, withoutScores(resp.Results))

	// A symlinked root is indexed through its target and reported
	// under the requested path.
	code, resp = requestFindFiles(t, handler, url.Values{"root": {link}, "q": {"widget"}})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, []workspacesdk.FindFilesResult{{
		AbsolutePath: filepath.Join(link, "deep", "deeper", "x_widget_extra_long_name.go"),
		Path:         "deep/deeper/x_widget_extra_long_name.go",
		Root:         link,
	}}, withoutScores(resp.Results))

	// Overlapping roots return each file once.
	code, resp = requestFindFiles(t, handler, url.Values{"root": {rootA, filepath.Join(rootA, "deep")}, "q": {"widget"}})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Results, 1)

	code, _ = requestFindFiles(t, handler, url.Values{"root": {"/"}, "q": {"widget"}})
	require.Equal(t, http.StatusBadRequest, code)
}

func TestHandleFindFiles_ConcurrentEviction(t *testing.T) {
	t.Parallel()

	const numRoots = 12
	home := t.TempDir()
	for i := range numRoots {
		p := filepath.Join(home, fmt.Sprintf("root%d", i), fmt.Sprintf("uniq%d_target.go", i))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, nil, 0o600))
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}))
	handler := api.Routes()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var ok, busy int
	for g := range 3 * numRoots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := g % numRoots
			for range 5 {
				code, resp := requestFindFiles(t, handler, url.Values{
					"root": {fmt.Sprintf("~/root%d", i)},
					"q":    {fmt.Sprintf("uniq%d_target", i)},
				})
				mu.Lock()
				switch code {
				case http.StatusOK:
					ok++
					assert.Len(t, resp.Results, 1, "root%d lost its match", i)
				case http.StatusServiceUnavailable:
					busy++
				default:
					assert.Fail(t, "unexpected status", "root%d: %d", i, code)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Positive(t, ok)
	t.Logf("ok=%d busy=%d", ok, busy)

	done := make(chan error, 1)
	go func() { done <- api.Close() }()
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func withoutScores(results []workspacesdk.FindFilesResult) []workspacesdk.FindFilesResult {
	out := make([]workspacesdk.FindFilesResult, len(results))
	for i, r := range results {
		r.Score = 0
		out[i] = r
	}
	return out
}

func TestHandleFindFiles_Gitignore(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	for _, rel := range []string{".git/HEAD", "src/ignorecheck_main.go", "dist/ignorecheck_bundle.js", "ignorecheck.log"} {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, nil, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("dist/\n*.log\n"), 0o600))

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, afero.NewOsFs(), nil, agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, api.Close()) })

	code, resp := requestFindFiles(t, api.Routes(), url.Values{"root": {repo}, "q": {"ignorecheck"}})
	require.Equal(t, http.StatusOK, code)
	paths := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		paths = append(paths, r.Path)
	}
	require.Equal(t, []string{"src/ignorecheck_main.go"}, paths)
}
