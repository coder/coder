package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cryptorand"
)

// nolint:paralleltest
func TestDotfiles(t *testing.T) {
	t.Run("MissingArg", func(t *testing.T) {
		cmd, _ := clitest.New(t, "dotfiles")
		err := cmd.Execute()
		require.Error(t, err)
	})
	t.Run("NoInstallScript", func(t *testing.T) {
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		// nolint:gosec
		err := os.WriteFile(filepath.Join(testRepo, ".bashrc"), []byte("wow"), 0o750)
		require.NoError(t, err)

		c := exec.Command("git", "add", ".bashrc")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add .bashrc"`)
		c.Dir = testRepo
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))

		cmd, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = cmd.Execute()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")
	})
	t.Run("InstallScript", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("install scripts on windows require sh and aren't very practical")
		}
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		// nolint:gosec
		err := os.WriteFile(filepath.Join(testRepo, "install.sh"), []byte("#!/bin/bash\necho wow > "+filepath.Join(string(root), ".bashrc")), 0o750)
		require.NoError(t, err)

		c := exec.Command("git", "add", "install.sh")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add install.sh"`)
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		cmd, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = cmd.Execute()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow\n")
	})
	t.Run("SymlinkBackup", func(t *testing.T) {
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		// nolint:gosec
		err := os.WriteFile(filepath.Join(testRepo, ".bashrc"), []byte("wow"), 0o750)
		require.NoError(t, err)

		// add a conflicting file at destination
		// nolint:gosec
		err = os.WriteFile(filepath.Join(string(root), ".bashrc"), []byte("backup"), 0o750)
		require.NoError(t, err)

		c := exec.Command("git", "add", ".bashrc")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add .bashrc"`)
		c.Dir = testRepo
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))

		cmd, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = cmd.Execute()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")

		// check for backup file
		b, err = os.ReadFile(filepath.Join(string(root), ".bashrc.bak"))
		require.NoError(t, err)
		require.Equal(t, string(b), "backup")
	})
}

func testGitRepo(t *testing.T, root config.Root) string {
	r, err := cryptorand.String(8)
	require.NoError(t, err)
	dir := filepath.Join(string(root), fmt.Sprintf("test-repo-%s", r))
	err = os.MkdirAll(dir, 0o750)
	require.NoError(t, err)

	c := exec.Command("git", "init")
	c.Dir = dir
	err = c.Run()
	require.NoError(t, err)

	c = exec.Command("git", "config", "user.email", "ci@coder.com")
	c.Dir = dir
	err = c.Run()
	require.NoError(t, err)

	c = exec.Command("git", "config", "user.name", "C I")
	c.Dir = dir
	err = c.Run()
	require.NoError(t, err)

	return dir
}
