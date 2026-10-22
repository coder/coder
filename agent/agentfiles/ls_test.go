package agentfiles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func requestLS(t *testing.T, handler http.Handler, path, body string) (int, workspacesdk.LSResponse) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	target := "/list-directory"
	if path != "" {
		target += "?path=" + url.QueryEscape(path)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	handler.ServeHTTP(w, r)

	var resp workspacesdk.LSResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	}
	return w.Code, resp
}

func lsNames(resp workspacesdk.LSResponse) []string {
	names := make([]string, 0, len(resp.Contents))
	for _, f := range resp.Contents {
		names = append(names, f.Name)
	}
	return names
}

func TestHandleLS_PathForms(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	workDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(home, "homesub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "homesub", "inhome.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, "home.txt"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "worksub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "worksub", "inwork.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "work.txt"), nil, 0o600))

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	newHandler := func(workingDir string) http.Handler {
		return agentfiles.NewAPI(logger, afero.NewOsFs(), nil,
			agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: home}),
			agentfiles.WithWorkingDir(func() string { return workingDir }),
		).Routes()
	}
	withWorkDir := newHandler(workDir)

	tests := []struct {
		name    string
		handler http.Handler
		path    string
		body    string
		wantDir string
		want    []string
	}{
		{name: "EmptyWithWorkingDir", handler: withWorkDir, wantDir: workDir, want: []string{"worksub", "work.txt"}},
		{name: "EmptyBodyObject", handler: withWorkDir, body: "{}", wantDir: workDir, want: []string{"worksub", "work.txt"}},
		{name: "EmptyWithoutWorkingDir", handler: newHandler(""), wantDir: home, want: []string{"homesub", "home.txt"}},
		{name: "EmptyWithMissingWorkingDir", handler: newHandler(filepath.Join(workDir, "missing")), wantDir: home, want: []string{"homesub", "home.txt"}},
		{name: "Tilde", handler: withWorkDir, path: "~", wantDir: home, want: []string{"homesub", "home.txt"}},
		{name: "TildeSub", handler: withWorkDir, path: "~/homesub", wantDir: filepath.Join(home, "homesub"), want: []string{"inhome.txt"}},
		{name: "Absolute", handler: withWorkDir, path: home + "/homesub/../homesub/", wantDir: filepath.Join(home, "homesub"), want: []string{"inhome.txt"}},
		{name: "Relative", handler: withWorkDir, path: "worksub", wantDir: filepath.Join(workDir, "worksub"), want: []string{"inwork.txt"}},
		{name: "UncleanWorkingDir", handler: newHandler(workDir + "/worksub/../"), wantDir: workDir, want: []string{"worksub", "work.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, resp := requestLS(t, tt.handler, tt.path, tt.body)
			require.Equal(t, http.StatusOK, code)
			require.Equal(t, tt.wantDir, resp.AbsolutePathString)
			require.Equal(t, tt.want, lsNames(resp))
			require.False(t, resp.Truncated)
		})
	}
}

func TestHandleLS_LegacyRelativityBody(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("root relativity uses drive letters on Windows")
	}

	dir := t.TempDir()
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "legacy.txt"), nil, 0o600))

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	handler := agentfiles.NewAPI(logger, afero.NewOsFs(), nil,
		agentfiles.WithWorkingDir(func() string { return workDir }),
	).Routes()

	parts := strings.FieldsFunc(dir, func(r rune) bool { return r == os.PathSeparator })
	body, err := json.Marshal(workspacesdk.LSRequest{
		Path:       parts,
		Relativity: workspacesdk.LSRelativityRoot,
	})
	require.NoError(t, err)

	code, resp := requestLS(t, handler, "", string(body))
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, dir, resp.AbsolutePathString)
	require.Equal(t, []string{"legacy.txt"}, lsNames(resp))

	// AgentConn.LS always sends the relativity key, so a relative path
	// with an otherwise empty request stays rejected.
	body, err = json.Marshal(workspacesdk.LSRequest{})
	require.NoError(t, err)
	code, _ = requestLS(t, handler, "relative", string(body))
	require.Equal(t, http.StatusInternalServerError, code)
}

func TestHandleLS_BodyTooLarge(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	handler := agentfiles.NewAPI(logger, afero.NewOsFs(), nil).Routes()

	body := `{"path":["` + strings.Repeat("a", 5<<20) + `"]}`
	code, _ := requestLS(t, handler, t.TempDir(), body)
	require.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func TestHandleLS_EntryMetadata(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "dirlink")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "broken")))

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	handler := agentfiles.NewAPI(logger, afero.NewOsFs(), nil).Routes()

	code, resp := requestLS(t, handler, dir, "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, []string{"dirlink", "target", "broken", "file.txt"}, lsNames(resp))

	byName := make(map[string]workspacesdk.LSFile)
	for _, f := range resp.Contents {
		byName[f.Name] = f
	}
	require.True(t, byName["dirlink"].IsDir)
	require.True(t, byName["dirlink"].IsSymlink)
	require.True(t, byName["target"].IsDir)
	require.False(t, byName["target"].IsSymlink)
	require.False(t, byName["broken"].IsDir)
	require.True(t, byName["broken"].IsSymlink)
	require.Equal(t, int64(5), byName["file.txt"].Size)

	info, err := os.Stat(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(byName["file.txt"].ModTime))
}

func TestHandleLS_Ignored(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(repo, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	write(".gitignore", "build/\n*.log\n!keep.log\n")
	write("build/out/x.bin", "")
	write("a.log", "")
	write("keep.log", "")
	write("main.go", "")
	write("inner/.git/HEAD", "")
	write("inner/a.log", "")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	handler := agentfiles.NewAPI(logger, afero.NewOsFs(), nil,
		agentfiles.WithEnvInfo(fakeBundleEnvInfo{home: t.TempDir()}),
	).Routes()
	ignored := func(dir string) map[string]bool {
		t.Helper()
		code, resp := requestLS(t, handler, dir, "")
		require.Equal(t, http.StatusOK, code)
		got := make(map[string]bool)
		for _, f := range resp.Contents {
			got[f.Name] = f.Ignored
		}
		return got
	}

	require.Equal(t, map[string]bool{
		".git": false, ".gitignore": false, "build": true, "inner": false,
		"a.log": true, "keep.log": false, "main.go": false,
	}, ignored(repo))
	// Entries inside an ignored directory are ignored.
	require.Equal(t, map[string]bool{"x.bin": true}, ignored(filepath.Join(repo, "build", "out")))
	// A nested repository uses its own rules.
	require.Equal(t, map[string]bool{".git": false, "a.log": false}, ignored(filepath.Join(repo, "inner")))

	// Changing .gitignore changes the result.
	write(".gitignore", "main.go\n")
	got := ignored(repo)
	require.True(t, got["main.go"])
	require.False(t, got["a.log"])

	// Outside any repository nothing is ignored.
	plain := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(plain, ".gitignore"), []byte("*\n"), 0o600))
	require.Equal(t, map[string]bool{".gitignore": false}, ignored(plain))
}

func TestHandleLS_NotADirectory(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "README.md")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	handler := agentfiles.NewAPI(logger, afero.NewOsFs(), nil).Routes()

	code, _ := requestLS(t, handler, file, "")
	require.Equal(t, http.StatusBadRequest, code)
}
