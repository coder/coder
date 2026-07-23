package usershell_test

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/usershell"
)

//nolint:paralleltest,tparallel // This test sets an environment variable.
func TestShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}

	ei := usershell.SystemEnvInfo{}

	t.Run("Fallback", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/sh")

		t.Run("NonExistentUser", func(t *testing.T) {
			shell, err := ei.Shell("notauser")
			require.NoError(t, err)
			require.Equal(t, "/bin/sh", shell)
		})
	})

	t.Run("NoFallback", func(t *testing.T) {
		// Disable env fallback for these tests.
		t.Setenv("SHELL", "")

		t.Run("NotFound", func(t *testing.T) {
			_, err := ei.Shell("notauser")
			require.Error(t, err)
		})

		t.Run("User", func(t *testing.T) {
			u, err := user.Current()
			require.NoError(t, err)
			shell, err := ei.Shell(u.Username)
			require.NoError(t, err)
			require.NotEmpty(t, shell)
		})
	})

	t.Run("Remove GOTRACEBACK=none", func(t *testing.T) {
		t.Setenv("GOTRACEBACK", "none")
		env := ei.Environ()
		for _, e := range env {
			require.NotEqual(t, "GOTRACEBACK=none", e)
		}
	})
}

// homeEnvInfo reports a fixed home directory and otherwise delegates to
// SystemEnvInfo, isolating ResolveWorkingDirectory tests from the host's real
// home directory.
type homeEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e homeEnvInfo) HomeDir() (string, error) { return e.home, nil }

// errorEnvInfo reports an error from HomeDir to exercise the fallback
// error path.
type errorEnvInfo struct {
	usershell.SystemEnvInfo
	err error
}

func (e errorEnvInfo) HomeDir() (string, error) { return "", e.err }

func TestResolveWorkingDirectory(t *testing.T) {
	t.Parallel()

	const home = "/home/coder"
	ei := homeEnvInfo{home: home}

	t.Run("Exists", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		require.NoError(t, fs.MkdirAll("/work", 0o700))
		dir, err := usershell.ResolveWorkingDirectory(fs, ei, "/work")
		require.NoError(t, err)
		require.Equal(t, "/work", dir)
	})

	t.Run("Missing", func(t *testing.T) {
		t.Parallel()
		dir, err := usershell.ResolveWorkingDirectory(afero.NewMemMapFs(), ei, "/work")
		require.NoError(t, err)
		require.Equal(t, home, dir)
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		dir, err := usershell.ResolveWorkingDirectory(afero.NewMemMapFs(), ei, "")
		require.NoError(t, err)
		require.Equal(t, home, dir)
	})

	t.Run("NotADirectory", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		require.NoError(t, afero.WriteFile(fs, "/work", []byte("file"), 0o600))
		dir, err := usershell.ResolveWorkingDirectory(fs, ei, "/work")
		require.NoError(t, err)
		require.Equal(t, home, dir)
	})

	t.Run("HomeDirError", func(t *testing.T) {
		t.Parallel()
		ei := errorEnvInfo{err: xerrors.New("no home")}
		_, err := usershell.ResolveWorkingDirectory(afero.NewMemMapFs(), ei, "")
		require.ErrorContains(t, err, "no home")
	})

	t.Run("Symlink", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		// MemMapFs cannot model symlinks. Use the real filesystem to
		// confirm Stat follows symlinks: a link to a directory is honored,
		// a link to a non-directory falls back to home.
		fs := afero.NewOsFs()
		base := t.TempDir()

		realDir := filepath.Join(base, "real")
		require.NoError(t, os.Mkdir(realDir, 0o700))
		linkToDir := filepath.Join(base, "link-dir")
		require.NoError(t, os.Symlink(realDir, linkToDir))
		dir, err := usershell.ResolveWorkingDirectory(fs, ei, linkToDir)
		require.NoError(t, err)
		require.Equal(t, linkToDir, dir, "symlink to a directory should be honored")

		realFile := filepath.Join(base, "file")
		require.NoError(t, os.WriteFile(realFile, []byte("x"), 0o600))
		linkToFile := filepath.Join(base, "link-file")
		require.NoError(t, os.Symlink(realFile, linkToFile))
		dir, err = usershell.ResolveWorkingDirectory(fs, ei, linkToFile)
		require.NoError(t, err)
		require.Equal(t, home, dir, "symlink to a non-directory should fall back to home")
	})
}
