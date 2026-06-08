package agenttest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/usershell"
)

// PromptMarker is the sentinel a deterministic test shell prints as its
// prompt once it is ready for input. It contains no ANSI and is matched as a
// substring, so a terminal wrapping it in control sequences does not defeat
// detection.
const PromptMarker = "::coder-shell-ready::"

type shellEnvInfoConfig struct {
	shell   string
	homeDir string
}

// ShellEnvInfoOption configures ShellEnvInfo.
type ShellEnvInfoOption func(*shellEnvInfoConfig)

// WithShell forces a named shell instead of the platform default. The name is
// resolved against PATH unless it is already an absolute path.
func WithShell(shell string) ShellEnvInfoOption {
	return func(c *shellEnvInfoConfig) { c.shell = shell }
}

// WithHomeDir overrides the isolated HOME directory. By default ShellEnvInfo
// uses t.TempDir().
func WithHomeDir(dir string) ShellEnvInfoOption {
	return func(c *shellEnvInfoConfig) { c.homeDir = dir }
}

// shellEnvInfo wraps usershell.SystemEnvInfo, overriding the shell, home
// directory, and environment so an SSH session runs a deterministic shell
// whose prompt is the PromptMarker.
type shellEnvInfo struct {
	usershell.SystemEnvInfo
	shell string
	home  string
	env   []string
}

func (e *shellEnvInfo) Shell(string) (string, error) { return e.shell, nil }
func (e *shellEnvInfo) HomeDir() (string, error)     { return e.home, nil }
func (e *shellEnvInfo) Environ() []string            { return slices.Clone(e.env) }

// ShellEnvInfo returns a usershell.EnvInfoer that forces a deterministic
// shell with the PromptMarker as its prompt, an isolated temp HOME, and the
// environment wired so the marker appears once the shell is ready for input.
//
// On POSIX the home directory's .profile sets PS1 to the marker framed by real
// newlines (dash does not interpret \n). On Windows the PROMPT environment
// variable carries the marker (cmd.exe renders PROMPT, $_ expands to CRLF).
func ShellEnvInfo(t testing.TB, opts ...ShellEnvInfoOption) usershell.EnvInfoer {
	t.Helper()

	var c shellEnvInfoConfig
	for _, opt := range opts {
		opt(&c)
	}

	home := c.homeDir
	if home == "" {
		home = t.TempDir()
	}

	shell, err := resolveShell(c.shell)
	require.NoError(t, err, "resolve shell")

	env := usershell.SystemEnvInfo{}.Environ()
	if runtime.GOOS == "windows" {
		env = setEnv(env, "PROMPT", PromptMarker+"$_")
	} else {
		env = setEnv(env, "HOME", home)
		writeProfile(t, home, shell)
	}

	return &shellEnvInfo{shell: shell, home: home, env: env}
}

// resolveShell returns the platform default when name is empty, the path
// unchanged when it is absolute, otherwise the PATH lookup of name.
func resolveShell(name string) (string, error) {
	if name == "" {
		if runtime.GOOS == "windows" {
			return "cmd.exe", nil
		}
		return "/bin/sh", nil
	}
	if filepath.IsAbs(name) {
		return name, nil
	}
	return exec.LookPath(name)
}

// writeProfile drops the login profile(s) that set the marker prompt. The PS1
// value is framed by real newline bytes because dash does not interpret \n.
// bash reads .bash_profile in preference to .profile, so it gets its own copy.
func writeProfile(t testing.TB, home, shell string) {
	t.Helper()

	profile := "PS1='\n" + PromptMarker + "\n'\n"
	err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o600)
	require.NoError(t, err, "write .profile")

	if strings.HasPrefix(filepath.Base(shell), "bash") {
		err = os.WriteFile(filepath.Join(home, ".bash_profile"), []byte(profile), 0o600)
		require.NoError(t, err, "write .bash_profile")
	}
}

// setEnv returns env with any existing key entry removed and key=value
// appended, so the value is unambiguous regardless of how the OS resolves
// duplicate keys.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return append(out, prefix+value)
}
