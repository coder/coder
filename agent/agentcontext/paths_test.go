package agentcontext_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// switchHomeEnv overrides the platform-specific environment
// variable consulted by os.UserHomeDir for the duration of the
// test. Windows reads USERPROFILE; Linux and macOS read HOME.
func switchHomeEnv(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("USERPROFILE", dir)
	default:
		t.Setenv("HOME", dir)
	}
}

func TestCanonicalizePath_AbsoluteCleansAndResolves(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := agentcontext.CanonicalizePath(filepath.Join(dir, "a", "..", "b"))
	require.NoError(t, err)
	// Path does not exist; EvalSymlinks fails. Result is
	// lexically cleaned: filepath.Clean drops the "..".
	require.Equal(t, filepath.Join(dir, "b"), got)
}

func TestCanonicalizePath_RelativeRejected(t *testing.T) {
	t.Parallel()
	_, err := agentcontext.CanonicalizePath("relative/path")
	require.Error(t, err)
}

//nolint:paralleltest,tparallel // Uses t.Setenv.
func TestCanonicalizePath_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	switchHomeEnv(t, home)
	got, err := agentcontext.CanonicalizePath("~/.coder")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".coder"), got)
}

//nolint:paralleltest,tparallel // Uses t.Setenv.
func TestCanonicalizePath_BareTildeExpandsToHome(t *testing.T) {
	home := t.TempDir()
	switchHomeEnv(t, home)
	got, err := agentcontext.CanonicalizePath("~")
	require.NoError(t, err)
	// Canonicalize the same home path through the function under
	// test so the comparison handles platform-specific behavior of
	// EvalSymlinks (Windows can fail to resolve directories that
	// Linux/macOS resolve cleanly).
	want, err := agentcontext.CanonicalizePath(home)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCanonicalizePathIn_ExpandsAgainstGivenHome(t *testing.T) {
	t.Parallel()
	home := testutil.TempDirResolved(t)
	got, err := agentcontext.CanonicalizePathIn(home, "~/.coder")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".coder"), got)

	_, err = agentcontext.CanonicalizePathIn(home, "relative/path")
	require.Error(t, err)
}

func TestCanonicalizePath_FollowsSymlinks(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink requires developer mode or admin on Windows")
	}
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	require.NoError(t, os.Symlink(realDir, link))

	got, err := agentcontext.CanonicalizePath(link)
	require.NoError(t, err)
	// On macOS the temp dir is itself symlinked; both realDir and got
	// pass through the same EvalSymlinks so they line up.
	want, err := filepath.EvalSymlinks(realDir)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestValidateSourcePath_RejectsParentSegments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Build /a/../b underneath a real allowed root so the path is
	// absolute on every platform. Validation must still reject the
	// embedded ".." segment before it ever touches allowedRoots.
	bad := filepath.Join(root, "a") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "b"
	err := agentcontext.ValidateSourcePath(bad, []string{root})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parent traversal")
}

func TestValidateSourcePath_AllowsInsideRoot(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDirResolved(t)
	child := filepath.Join(dir, "child")
	require.NoError(t, os.MkdirAll(child, 0o755))

	require.NoError(t, agentcontext.ValidateSourcePath(child, []string{dir}))
	require.NoError(t, agentcontext.ValidateSourcePath(dir, []string{dir}))
}

func TestValidateSourcePath_RejectsOutsideRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	other := t.TempDir()
	err := agentcontext.ValidateSourcePath(other, []string{root})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not inside any allowed root")
}

func TestValidateSourcePath_EmptyAllowedRootsBypass(t *testing.T) {
	t.Parallel()
	require.NoError(t, agentcontext.ValidateSourcePath("/anywhere", nil))
}

func TestValidateSourcePath_InvalidRootsFailClosed(t *testing.T) {
	t.Parallel()
	// All allowed roots are relative and therefore invalid;
	// validation must fail closed.
	err := agentcontext.ValidateSourcePath("/anywhere", []string{"relative-only"})
	require.Error(t, err)
}

func TestValidateSourcePath_PathPrefixIsPathAware(t *testing.T) {
	t.Parallel()
	// "/a-prefix" is not inside "/a", even though it starts
	// with the same bytes.
	dir := t.TempDir()
	sibling := strings.TrimRight(dir, string(os.PathSeparator)) + "-sibling"
	require.NoError(t, os.MkdirAll(sibling, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	err := agentcontext.ValidateSourcePath(sibling, []string{dir})
	require.Error(t, err)
}
