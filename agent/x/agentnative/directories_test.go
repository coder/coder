//go:build linux

package agentnative_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentnative"
)

func prerequisiteDirectories(state, home, codex string) []string {
	return []string{
		state,
		filepath.Join(home, ".claude"),
		codex,
		filepath.Join(state, "opencode-home"),
		filepath.Join(state, "opencode-home", "config"),
		filepath.Join(state, "opencode-home", "data"),
		filepath.Join(state, "opencode-home", "state"),
		filepath.Join(state, "opencode-home", "cache"),
		filepath.Join(state, "opencode-home", "tmp"),
	}
}

func TestPrepareDirectories(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"default", "overrides", "empty-overrides", "duplicates", "last-empty"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			home, state := filepath.Join(root, "home"), filepath.Join(root, "state")
			codex := filepath.Join(home, ".codex")
			environment := []string{"HOME=" + home}
			switch scenario {
			case "overrides":
				codex = filepath.Join(root, "codex")
				environment = append(environment, "CODEX_HOME="+codex, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"))
			case "empty-overrides":
				environment = append(environment, "CODEX_HOME=", "CLAUDE_CONFIG_DIR=")
			case "duplicates":
				codex = filepath.Join(root, "codex")
				environment = []string{"HOME=relative", "CODEX_HOME=relative", "CLAUDE_CONFIG_DIR=relative", "HOME=" + home, "CODEX_HOME=" + codex, "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude")}
			case "last-empty":
				environment = append(environment, "CODEX_HOME=relative", "CLAUDE_CONFIG_DIR=relative", "CODEX_HOME=", "CLAUDE_CONFIG_DIR=")
			}
			// OpenCode uses its dedicated roots, not workspace XDG overrides.
			environment = append(environment, "XDG_CONFIG_HOME=relative", "XDG_DATA_HOME=relative", "XDG_STATE_HOME=relative", "XDG_CACHE_HOME=relative", "TMPDIR=relative", "OPENCODE_CONFIG_DIR=relative")
			original := slices.Clone(environment)
			require.NoError(t, agentnative.PrepareDirectories(state, environment))
			require.Equal(t, original, environment)
			paths := prerequisiteDirectories(state, home, codex)
			for _, path := range paths {
				info, err := os.Lstat(path)
				require.NoError(t, err)
				require.True(t, info.IsDir(), path)
				require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), path)
			}
			var actual []string
			require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				require.NoError(t, err)
				require.True(t, entry.IsDir(), "preparation must not create any files: %s", path)
				if path != root && path != home {
					actual = append(actual, path)
				}
				return nil
			}))
			require.ElementsMatch(t, paths, actual, "do not precreate native history, registry, or database directories")
		})
	}
}

func TestPrepareDirectoriesPreservesExistingState(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state := filepath.Join(home, "manager")
	codex := filepath.Join(home, ".codex")
	environment := []string{"HOME=" + home}
	require.NoError(t, agentnative.PrepareDirectories(state, environment))
	paths := prerequisiteDirectories(state, home, codex)
	files := []string{
		filepath.Join(state, "config.json"),
		filepath.Join(state, "sessions.json"),
		filepath.Join(state, "sessions.json.tmp"),
		filepath.Join(state, "manager.json"),
		filepath.Join(state, "codex-server.json"),
		filepath.Join(state, "opencode-server.json"),
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "agents", "registry.json"),
		filepath.Join(codex, "auth.json"),
		filepath.Join(codex, "config.toml"),
		filepath.Join(codex, "sessions", "history.jsonl"),
		filepath.Join(state, "opencode-home", "data", "opencode", "auth.json"),
		filepath.Join(state, "opencode-home", "data", "opencode", "opencode.db"),
	}
	for _, path := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("preserve "+path), 0o600))
	}
	link := filepath.Join(home, ".claude", "history.jsonl")
	require.NoError(t, os.Symlink(files[0], link))
	paths = append(paths, files...)
	paths = append(paths, link)
	before := make(map[string]os.FileInfo)
	for _, path := range paths {
		info, err := os.Lstat(path)
		require.NoError(t, err)
		before[path] = info
	}
	for range 2 {
		require.NoError(t, agentnative.PrepareDirectories(state, environment))
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		require.NoError(t, err)
		require.True(t, os.SameFile(before[path], info), path)
		require.Equal(t, before[path].Mode(), info.Mode(), path)
		require.Equal(t, before[path].ModTime(), info.ModTime(), path)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "preserve "+path, string(data))
	}
	entries, err := os.ReadDir(state)
	require.NoError(t, err)
	require.Len(t, entries, 7, "preparation must not acquire a manager lock or write any files")
}

func TestPrepareDirectoriesRejectsInvalidPaths(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"missing-home", "empty-home", "relative-home", "unclean-home", "nul-home", "long-home", "last-home-empty",
		"empty-state", "relative-state", "unclean-state", "nul-state", "long-state",
		"relative-codex", "unclean-codex", "nul-codex", "long-codex", "last-codex-relative",
		"custom-claude", "relative-claude", "last-claude-custom",
	} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			state := filepath.Join(home, "state")
			originalState := state
			environment := []string{"HOME=" + home}
			switch scenario {
			case "missing-home":
				environment = nil
			case "empty-home":
				environment = []string{"HOME="}
			case "relative-home":
				environment = []string{"HOME=relative"}
			case "unclean-home":
				environment = []string{"HOME=" + home + "/../home"}
			case "nul-home":
				environment = []string{"HOME=" + home + "\x00"}
			case "long-home":
				environment = []string{"HOME=/" + strings.Repeat("a", 4096)}
			case "last-home-empty":
				environment = append(environment, "HOME=")
			case "empty-state":
				state = ""
			case "relative-state":
				state = "relative"
			case "unclean-state":
				state += "/../state"
			case "nul-state":
				state += "\x00"
			case "long-state":
				state = "/" + strings.Repeat("a", 4096)
			case "relative-codex":
				environment = append(environment, "CODEX_HOME=relative")
			case "unclean-codex":
				environment = append(environment, "CODEX_HOME="+home+"/../codex")
			case "nul-codex":
				environment = append(environment, "CODEX_HOME="+home+"\x00")
			case "long-codex":
				environment = append(environment, "CODEX_HOME=/"+strings.Repeat("a", 4096))
			case "last-codex-relative":
				environment = append(environment, "CODEX_HOME="+filepath.Join(home, ".codex"), "CODEX_HOME=relative")
			case "custom-claude":
				environment = append(environment, "CLAUDE_CONFIG_DIR="+filepath.Join(home, "custom"))
			case "relative-claude":
				environment = append(environment, "CLAUDE_CONFIG_DIR=relative")
			case "last-claude-custom":
				environment = append(environment, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"), "CLAUDE_CONFIG_DIR="+filepath.Join(home, "custom"))
			}
			require.Error(t, agentnative.PrepareDirectories(state, environment))
			require.NoDirExists(t, originalState, "validate path resolution before filesystem changes")
			require.NoDirExists(t, filepath.Join(home, ".claude"))
			require.NoDirExists(t, filepath.Join(home, ".codex"))
		})
	}
}

func TestPrepareDirectoriesRejectsUnsafeDirectories(t *testing.T) {
	t.Parallel()
	for _, index := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		for _, scenario := range []string{"symlink", "file", "permissions", "writable-parent", "parent-symlink"} {
			name := []string{"manager", "claude", "codex", "opencode", "config", "data", "state", "cache", "tmp"}[index]
			t.Run(name+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				home := filepath.Join(root, "workspace")
				state := filepath.Join(root, "manager", "state")
				codex := filepath.Join(root, "codex", "home")
				environment := []string{"HOME=" + home, "CODEX_HOME=" + codex}
				path := prerequisiteDirectories(state, home, codex)[index]
				parent := filepath.Dir(path)
				target := t.TempDir()
				sentinel := filepath.Join(target, "auth.json")
				require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0o600))
				if scenario == "parent-symlink" {
					require.NoError(t, os.MkdirAll(filepath.Dir(parent), 0o700))
					require.NoError(t, os.Symlink(target, parent))
				} else {
					require.NoError(t, os.MkdirAll(parent, 0o700))
					switch scenario {
					case "symlink":
						require.NoError(t, os.Symlink(target, path))
					case "file":
						require.NoError(t, os.WriteFile(path, []byte("preserve"), 0o600))
					case "permissions":
						require.NoError(t, os.Mkdir(path, 0o700))
						// #nosec G302 -- Unsafe permissions deliberately exercise rejection.
						require.NoError(t, os.Chmod(path, 0o755))
					case "writable-parent":
						// #nosec G302 -- Unsafe permissions deliberately exercise rejection.
						require.NoError(t, os.Chmod(parent, 0o777))
					}
				}
				preserved := path
				if scenario == "parent-symlink" || scenario == "writable-parent" {
					preserved = parent
				}
				before, err := os.Lstat(preserved)
				require.NoError(t, err)
				require.Error(t, agentnative.PrepareDirectories(state, environment))
				after, err := os.Lstat(preserved)
				require.NoError(t, err)
				require.True(t, os.SameFile(before, after))
				require.Equal(t, before.Mode(), after.Mode(), "must not repair existing permissions")
				if scenario == "file" {
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, "preserve", string(data))
				}
				data, err := os.ReadFile(sentinel)
				require.NoError(t, err)
				require.Equal(t, "preserve", string(data))
				entries, err := os.ReadDir(target)
				require.NoError(t, err)
				require.Len(t, entries, 1, "must not create through a symlink")
			})
		}
	}
}
