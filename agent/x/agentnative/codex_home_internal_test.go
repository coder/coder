//go:build linux

package agentnative

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexCreateSetupFailure(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	f.codex.options.Environment = []string{"CODEX_HOME=relative"}
	f.codex.dial = func(context.Context, func(codexMessage), func()) (codexRPC, error) {
		return nil, errCodexSocketClosed
	}
	_, err := f.codex.Create(t.Context(), f.session.Cwd)
	var setup *creationSetupError
	require.ErrorAs(t, err, &setup)
	require.Contains(t, err.Error(), "requires an absolute CODEX_HOME")
	require.Empty(t, f.rpc.methods(), "setup failure must precede thread/start")
	require.Nil(t, f.codex.owned)
}

func TestCodexCreateDispatchedFailure(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	handler := f.rpc.handler
	f.rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "thread/start" {
			return nil, &codexRPCError{}
		}
		return handler(method, params)
	}
	_, err := f.codex.Create(t.Context(), f.session.Cwd)
	require.Error(t, err)
	var setup *creationSetupError
	require.False(t, errors.As(err, &setup), "a dispatched request cannot be classified as a setup failure")
	require.Equal(t, []string{"initialize", "thread/start"}, f.rpc.methods())
}

func TestCodexServerPreparesHome(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"configured", "default", "empty", "existing"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			c := newCodexFixture(t).codex
			home := t.TempDir()
			codexHome := filepath.Join(home, ".codex")
			c.options.Environment = []string{"HOME=" + home}
			switch scenario {
			case "configured":
				codexHome = filepath.Join(t.TempDir(), "nested", "codex")
				c.options.Environment = []string{"HOME=relative", "CODEX_HOME=" + codexHome}
			case "empty":
				c.options.Environment = append(c.options.Environment, "CODEX_HOME=")
			case "existing":
				require.NoError(t, os.Mkdir(codexHome, 0o700))
				c.options.Environment = append(c.options.Environment, "CODEX_HOME="+codexHome)
			}
			files := map[string][]byte{
				"auth.json":   []byte(`{"test":"preserve existing authentication"}`),
				"config.toml": []byte("model = \"preserve-existing-model\"\n"),
			}
			before := make(map[string]os.FileInfo)
			if scenario == "existing" {
				for name, data := range files {
					path := filepath.Join(codexHome, name)
					require.NoError(t, os.WriteFile(path, data, 0o600))
					info, err := os.Stat(path)
					require.NoError(t, err)
					before[name] = info
				}
			} else {
				require.NoDirExists(t, codexHome)
			}
			c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				info, err := os.Stat(codexHome)
				require.NoError(t, err, "home must exist before starting the native process")
				require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
				return exec.CommandContext(ctx, "sleep", "3600")
			}
			codexStartTestServer(t, c)
			t.Cleanup(func() { _ = c.killOwnedGroup() })
			if scenario == "configured" {
				require.NoDirExists(t, filepath.Join(home, ".codex"), "explicit CODEX_HOME must take precedence")
			}
			for name, data := range files {
				path := filepath.Join(codexHome, name)
				if scenario != "existing" {
					require.NoFileExists(t, path, "startup must not provision authentication or configuration")
					continue
				}
				actual, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, data, actual)
				info, err := os.Stat(path)
				require.NoError(t, err)
				require.True(t, os.SameFile(before[name], info))
				require.Equal(t, before[name].ModTime(), info.ModTime())
				require.Equal(t, before[name].Mode(), info.Mode())
			}
		})
	}
}

func TestCodexServerRejectsUnsafeHome(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"relative", "missing-home", "relative-home", "symlink", "parent-symlink", "permissions", "parent-permissions", "file"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			c := newCodexFixture(t).codex
			home := t.TempDir()
			codexHome := filepath.Join(home, ".codex")
			c.options.Environment = []string{"HOME=" + home, "CODEX_HOME=" + codexHome}
			target := t.TempDir()
			preserved := filepath.Join(target, "auth.json")
			require.NoError(t, os.WriteFile(preserved, []byte("preserve"), 0o600))
			switch scenario {
			case "relative":
				c.options.Environment = []string{"HOME=" + home, "CODEX_HOME=relative"}
			case "missing-home":
				c.options.Environment = nil
			case "relative-home":
				c.options.Environment = []string{"HOME=relative"}
			case "symlink":
				require.NoError(t, os.Symlink(target, codexHome))
			case "parent-symlink":
				parent := filepath.Join(home, "link")
				require.NoError(t, os.Symlink(target, parent))
				c.options.Environment = []string{"CODEX_HOME=" + filepath.Join(parent, "child")}
			case "permissions":
				require.NoError(t, os.Mkdir(codexHome, 0o700))
				// #nosec G302 -- Unsafe permissions deliberately exercise rejection.
				require.NoError(t, os.Chmod(codexHome, 0o755))
			case "parent-permissions":
				// #nosec G302 -- Unsafe permissions deliberately exercise rejection.
				require.NoError(t, os.Chmod(home, 0o777))
			case "file":
				require.NoError(t, os.WriteFile(codexHome, []byte("preserve"), 0o600))
			}
			require.Error(t, c.startServer())
			require.Nil(t, c.owned)
			require.NoFileExists(t, filepath.Join(c.options.StateDir, "codex-server.log"))
			data, err := os.ReadFile(preserved)
			require.NoError(t, err)
			require.Equal(t, "preserve", string(data))
			require.NoDirExists(t, filepath.Join(target, "child"), "must not create through a parent symlink")
			switch scenario {
			case "permissions":
				info, err := os.Stat(codexHome)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "must not repair existing permissions")
			case "parent-permissions":
				require.NoDirExists(t, codexHome, "must reject an unsafe parent before creating descendants")
			case "file":
				data, err := os.ReadFile(codexHome)
				require.NoError(t, err)
				require.Equal(t, "preserve", string(data))
			}
		})
	}
}
