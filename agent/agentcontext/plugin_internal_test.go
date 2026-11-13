package agentcontext

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasLinkInside(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "link")))
	canonReal, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)

	canonDir := filepath.Dir(canonReal)

	assert.False(t, hasLinkInside(canonDir, filepath.Join(canonReal, "sub")))
	assert.True(t, hasLinkInside(canonDir, filepath.Join(canonDir, "link", "sub")), "symlinked ancestor below base")
	assert.True(t, hasLinkInside("/", filepath.Join(canonDir, "link", "sub")), "symlinked ancestor below a root base")
	assert.True(t, hasLinkInside(canonDir, filepath.Join(canonReal, "missing")), "unreadable component")
	assert.True(t, hasLinkInside(filepath.Join(canonDir, "link"), filepath.Join(canonDir, "link")), "base that is itself a link")
	linkedSub := filepath.Join(canonDir, "link", "sub")
	assert.False(t, hasLinkInside(linkedSub, linkedSub), "links above base are trusted")
}

func TestHasLinkInside_Junction(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("junctions exist only on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	link := filepath.Join(dir, "link")
	// Creating a junction needs no elevation, unlike a symlink.
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	require.NoError(t, err, string(out))

	assert.False(t, hasLinkInside(dir, filepath.Join(target, "sub")))
	assert.True(t, hasLinkInside(dir, filepath.Join(link, "sub")), "junction below base")
	assert.True(t, hasLinkInside(filepath.VolumeName(dir)+`\`, filepath.Join(link, "sub")), "junction below a drive root base")
}
