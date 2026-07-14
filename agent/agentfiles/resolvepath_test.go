package agentfiles_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolvePath_FollowsFileSymlink(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	realPath := filepath.Join(dir, "real.txt")
	err := afero.WriteFile(osFs, realPath, []byte("hello"), 0o644)
	require.NoError(t, err)

	linkPath := filepath.Join(dir, "link.txt")
	err = os.Symlink(realPath, linkPath)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/resolve-path?path=%s", linkPath), nil)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var resp workspacesdk.ResolvePathResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, mustEvalSymlinks(t, realPath), resp.ResolvedPath)
}

func TestResolvePath_FollowsSymlinkedParentForMissingFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	realPlansDir := filepath.Join(dir, "real-plans")
	err := os.MkdirAll(realPlansDir, 0o755)
	require.NoError(t, err)

	linkPlansDir := filepath.Join(dir, "link-plans")
	err = os.Symlink(realPlansDir, linkPlansDir)
	require.NoError(t, err)

	requestedPath := filepath.Join(linkPlansDir, "PLAN.md")
	resolvedPath := filepath.Join(mustEvalSymlinks(t, realPlansDir), "PLAN.md")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/resolve-path?path=%s", requestedPath), nil)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var resp workspacesdk.ResolvePathResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, resolvedPath, resp.ResolvedPath)
}

func TestResolvePath_FollowsSymlinkedParentForExistingFile(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	realPlansDir := filepath.Join(dir, "real-plans")
	err := os.MkdirAll(realPlansDir, 0o755)
	require.NoError(t, err)

	resolvedPath := filepath.Join(realPlansDir, "PLAN.md")
	err = afero.WriteFile(osFs, resolvedPath, []byte("plan"), 0o644)
	require.NoError(t, err)

	linkPlansDir := filepath.Join(dir, "link-plans")
	err = os.Symlink(realPlansDir, linkPlansDir)
	require.NoError(t, err)

	requestedPath := filepath.Join(linkPlansDir, "PLAN.md")

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/resolve-path?path=%s", requestedPath), nil)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var resp workspacesdk.ResolvePathResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, mustEvalSymlinks(t, resolvedPath), resp.ResolvedPath)
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolvedPath, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolvedPath
}
