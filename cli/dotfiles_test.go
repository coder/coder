package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cryptorand"
)

func TestDotfiles(t *testing.T) {
	t.Parallel()
	// This test will time out if the user has commit signing enabled.
	if _, gpgTTYFound := os.LookupEnv("GPG_TTY"); gpgTTYFound {
		t.Skip("GPG_TTY is set, skipping test to avoid hanging")
	}
	t.Run("MissingArg", func(t *testing.T) {
		t.Parallel()
		inv, _ := clitest.New(t, "dotfiles")
		err := inv.Run()
		require.Error(t, err)
	})
	t.Run("NoInstallScript", func(t *testing.T) {
		t.Parallel()
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

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")
	})
	t.Run("SwitchRepoDir", func(t *testing.T) {
		t.Parallel()
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

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "--repo-dir", "testrepo", "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")

		stat, staterr := os.Stat(filepath.Join(string(root), "testrepo"))
		require.NoError(t, staterr)
		require.True(t, stat.IsDir())
	})
	t.Run("SwitchRepoDirRelative", func(t *testing.T) {
		t.Parallel()
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

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "--repo-dir", "./relrepo", "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")

		stat, staterr := os.Stat(filepath.Join(string(root), "relrepo"))
		require.NoError(t, staterr)
		require.True(t, stat.IsDir())
	})
	t.Run("SymlinkBackup", func(t *testing.T) {
		t.Parallel()
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

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")

		// check for backup file
		b, err = os.ReadFile(filepath.Join(string(root), ".bashrc.bak"))
		require.NoError(t, err)
		require.Equal(t, string(b), "backup")

		// check for idempotency
		inv, _ = clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)
		b, err = os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow")
		b, err = os.ReadFile(filepath.Join(string(root), ".bashrc.bak"))
		require.NoError(t, err)
		require.Equal(t, string(b), "backup")
	})
}

func TestDotfilesInstallScriptUnix(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip()
	}

	t.Run("InstallScript", func(t *testing.T) {
		t.Parallel()
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

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow\n")
	})

	t.Run("NestedInstallScript", func(t *testing.T) {
		t.Parallel()
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		scriptPath := filepath.Join("script", "setup")
		err := os.MkdirAll(filepath.Join(testRepo, "script"), 0o750)
		require.NoError(t, err)
		// nolint:gosec
		err = os.WriteFile(filepath.Join(testRepo, scriptPath), []byte("#!/bin/bash\necho wow > "+filepath.Join(string(root), ".bashrc")), 0o750)
		require.NoError(t, err)

		c := exec.Command("git", "add", scriptPath)
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add script"`)
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow\n")
	})

	t.Run("InstallScriptChangeBranch", func(t *testing.T) {
		t.Parallel()
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		// We need an initial commit to start the `main` branch
		c := exec.Command("git", "commit", "--allow-empty", "-m", `"initial commit"`)
		c.Dir = testRepo
		err := c.Run()
		require.NoError(t, err)

		// nolint:gosec
		err = os.WriteFile(filepath.Join(testRepo, "install.sh"), []byte("#!/bin/bash\necho wow > "+filepath.Join(string(root), ".bashrc")), 0o750)
		require.NoError(t, err)

		c = exec.Command("git", "checkout", "-b", "other_branch")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "add", "install.sh")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add install.sh"`)
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "checkout", "main")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo, "-b", "other_branch")
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, string(b), "wow\n")
	})
}

func TestDotfilesInstallScriptWindows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip()
	}

	t.Run("InstallScript", func(t *testing.T) {
		t.Parallel()
		_, root := clitest.New(t)
		testRepo := testGitRepo(t, root)

		// nolint:gosec
		err := os.WriteFile(filepath.Join(testRepo, "install.ps1"), []byte("echo \"hello, computer!\" > "+filepath.Join(string(root), "greeting.txt")), 0o750)
		require.NoError(t, err)

		c := exec.Command("git", "add", "install.ps1")
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		c = exec.Command("git", "commit", "-m", `"add install.ps1"`)
		c.Dir = testRepo
		err = c.Run()
		require.NoError(t, err)

		inv, _ := clitest.New(t, "dotfiles", "--global-config", string(root), "--symlink-dir", string(root), "-y", testRepo)
		err = inv.Run()
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(string(root), "greeting.txt"))
		require.NoError(t, err)
		require.Equal(t, string(b), utf16.Encode([]rune("hello, computer!\n")))
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

	c = exec.Command("git", "checkout", "-b", "main")
	c.Dir = dir
	err = c.Run()
	require.NoError(t, err)

	return dir
}
