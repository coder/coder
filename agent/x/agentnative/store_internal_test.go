//go:build linux

package agentnative

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestManagerPrivateStorage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "archive-symlink", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(dir, "sessions.json")))
		}},
		{name: "archive-hardlink", setup: func(t *testing.T, dir string) {
			file := filepath.Join(dir, "original")
			require.NoError(t, os.WriteFile(file, []byte(`{"version":1,"sessions":{}}`), 0o600))
			require.NoError(t, os.Link(file, filepath.Join(dir, "sessions.json")))
		}},
		{name: "archive-permissions", setup: func(t *testing.T, dir string) {
			file := filepath.Join(dir, "sessions.json")
			require.NoError(t, os.WriteFile(file, []byte(`{"version":1,"sessions":{}}`), 0o600))
			require.NoError(t, os.Chmod(file, 0o644))
		}},
		{name: "archive-fifo", setup: func(t *testing.T, dir string) {
			require.NoError(t, unix.Mkfifo(filepath.Join(dir, "sessions.json"), 0o600))
		}},
		{name: "lock-symlink", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(dir, "manager.lock")))
		}},
		{name: "corrupt-archive", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(`{"version":`), 0o600))
		}},
		{name: "temporary-symlink", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(dir, "sessions.json.tmp")))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "state")
			require.NoError(t, os.Mkdir(dir, 0o700))
			test.setup(t, dir)
			_, err := New(Options{StateDir: dir, Factories: map[string]func(BackendOptions) (Backend, error){}})
			require.Error(t, err)
		})
	}
}

func TestManagerDirectoryAndExclusiveLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	m := newTestManager(t, dir, &managerTestBackend{})
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	for _, name := range []string{"sessions.json", "manager.json", "manager.lock"} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	_, err = New(Options{StateDir: dir, Factories: map[string]func(BackendOptions) (Backend, error){}})
	require.ErrorContains(t, err, "already running")
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(dir, link))
	_, err = New(Options{StateDir: link, Factories: map[string]func(BackendOptions) (Backend, error){}})
	require.Error(t, err)
	// Model a child retaining the open file description between fork and exec.
	duplicate, err := syscall.Dup(int(m.store.dir.lock.Fd()))
	require.NoError(t, err)
	defer syscall.Close(duplicate)
	require.NoError(t, m.Close())
	reopened := newTestManager(t, dir, &managerTestBackend{})
	require.NoError(t, reopened.Close())
}

func TestManagerInterruptedTemporaryIsNotPromoted(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(`{"version":1,"sessions":{}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sessions.json.tmp"), []byte("interrupted write"), 0o600))
	m := newTestManager(t, dir, &managerTestBackend{})
	require.Empty(t, m.List())
	_, err := os.Stat(filepath.Join(dir, "sessions.json.tmp"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestNativeConfiguration(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	config, err := ReadConfig(dir)
	require.NoError(t, err)
	require.Nil(t, config.InitialJob)
	config.InitialJob = &JobSpec{Harness: "codex", Cwd: t.TempDir(), Prompt: "literal initial prompt"}
	require.NoError(t, WriteConfig(dir, config))
	got, err := ReadConfig(dir)
	require.NoError(t, err)
	require.Equal(t, config, got)
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	m := newTestManager(t, dir, &managerTestBackend{})
	require.Error(t, WriteConfig(dir, Config{}))
	require.NoError(t, m.Close())
	require.Equal(t, filepath.Join("/home/user", ".local", "state", "coder-native"), DefaultStateDir("/home/user"))
}
