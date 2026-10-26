//go:build linux

package agentnative

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

func TestManagerPreparesDefaultDirectories(t *testing.T) {
	t.Parallel()
	for _, defaultState := range []bool{false, true} {
		name := "ExplicitState"
		if defaultState {
			name = "DefaultState"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			state := filepath.Join(home, "manager")
			if defaultState {
				state = ""
			}
			var commands atomic.Int32
			m, err := New(Options{
				StateDir: state, Environment: []string{"HOME=relative", "HOME=" + home, "PATH="}, Clock: quartz.NewMock(t),
				Command: func(context.Context, string, ...string) *exec.Cmd {
					commands.Add(1)
					return nil
				},
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })
			if defaultState {
				state = DefaultStateDir(home)
			}
			paths := []string{state, filepath.Join(home, ".claude"), filepath.Join(home, ".codex")}
			paths = append(paths, openCodeHomeDirectories(filepath.Join(state, "opencode-home"))...)
			for _, path := range paths {
				info, err := os.Stat(path)
				require.NoError(t, err, "directory must exist when the manager becomes available: %s", path)
				require.True(t, info.IsDir())
				require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
			}
			require.Empty(t, m.List())
			require.Len(t, m.backends, 3)
			require.False(t, m.backends["claude"].(*Claude).installed)
			require.Nil(t, m.backends["codex"].(*Codex).owned)
			require.False(t, m.backends["opencode"].(*OpenCode).started)
			_, err = ReadDescriptor(state)
			require.NoError(t, err)
			require.NoError(t, m.Close())
			require.Zero(t, commands.Load(), "startup and passive close must not probe or start native processes")
			for _, path := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".codex")} {
				entries, err := os.ReadDir(path)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
			entries, err := os.ReadDir(state)
			require.NoError(t, err)
			require.Len(t, entries, 4, "only the manager journal, descriptor, lock, and OpenCode home may exist")
		})
	}
}

func TestManagerCustomFactoriesOwnDirectories(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"empty", "custom", "missing-home"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			state := filepath.Join(home, "state")
			claude := filepath.Join(home, ".claude")
			require.NoError(t, os.WriteFile(claude, []byte("do not touch"), 0o600))
			environment := []string{"HOME=" + home, "CODEX_HOME=relative", "CLAUDE_CONFIG_DIR=unsupported"}
			factories := map[string]func(BackendOptions) (Backend, error){}
			var calls int
			switch scenario {
			case "custom":
				factories["claude"] = func(options BackendOptions) (Backend, error) {
					calls++
					require.Equal(t, environment, options.Environment)
					return &managerTestBackend{host: options.Host}, nil
				}
			case "missing-home":
				environment = nil
			}
			m, err := New(Options{StateDir: state, Environment: environment, Clock: quartz.NewMock(t), Factories: factories})
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })
			require.Len(t, m.backends, calls)
			if scenario == "custom" {
				require.Equal(t, 1, calls)
			}
			data, err := os.ReadFile(claude)
			require.NoError(t, err)
			require.Equal(t, "do not touch", string(data))
			require.NoDirExists(t, filepath.Join(home, ".codex"))
			require.NoDirExists(t, filepath.Join(state, "opencode-home"))
		})
	}
}

func TestManagerDirectoryPreparationPrecedesJournalWrites(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state := filepath.Join(home, "state")
	require.NoError(t, os.Mkdir(state, 0o700))
	files := map[string]string{
		"sessions.json":     `{"version":1,"sessions":{}}`,
		"sessions.json.tmp": "interrupted journal",
		"manager.json":      "previous descriptor",
	}
	before := make(map[string]os.FileInfo)
	for name, data := range files {
		path := filepath.Join(state, name)
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		info, err := os.Stat(path)
		require.NoError(t, err)
		before[name] = info
	}
	// A later backend prerequisite must fail before replacing the journal or
	// descriptor, including cleanup of an interrupted journal write.
	codex := filepath.Join(home, ".codex")
	require.NoError(t, os.WriteFile(codex, []byte("not a directory"), 0o600))
	m, err := New(Options{StateDir: state, Environment: []string{"HOME=" + home}, Clock: quartz.NewMock(t)})
	require.ErrorContains(t, err, "prepare native directory")
	require.Nil(t, m)
	for name, expected := range files {
		path := filepath.Join(state, name)
		actual, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, expected, string(actual))
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, os.SameFile(before[name], info))
		require.Equal(t, before[name].ModTime(), info.ModTime())
	}
	dir, err := openPrivateDirectory(state, privateDirectoryExisting)
	require.NoError(t, err)
	defer dir.close()
	require.NoError(t, dir.acquire(), "failed preparation must release the manager lock")
}

func TestManagerDirectoryPreparationFollowsLock(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state := filepath.Join(home, "state")
	m := newTestManager(t, state, &managerTestBackend{})
	other, err := New(Options{StateDir: state, Environment: []string{"HOME=" + home}, Clock: quartz.NewMock(t)})
	require.ErrorContains(t, err, "already running")
	require.Nil(t, other)
	require.NoDirExists(t, filepath.Join(home, ".claude"))
	require.NoDirExists(t, filepath.Join(home, ".codex"))
	require.NoDirExists(t, filepath.Join(state, "opencode-home"))
	require.NoError(t, m.Close())
}
