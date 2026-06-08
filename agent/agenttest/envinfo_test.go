package agenttest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
)

// TestShellEnvInfo asserts the deterministic-shell EnvInfoer forces a known
// shell, isolates HOME to a temp dir, and wires the sentinel prompt so the
// marker appears once the shell is ready for input.
func TestShellEnvInfo(t *testing.T) {
	t.Parallel()

	ei := agenttest.ShellEnvInfo(t)

	// HOME is an isolated, existing directory.
	home, err := ei.HomeDir()
	require.NoError(t, err)
	fi, err := os.Stat(home)
	require.NoError(t, err)
	require.True(t, fi.IsDir(), "home dir should exist")

	// The forced shell is deterministic per platform.
	shell, err := ei.Shell("")
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		require.Equal(t, "cmd.exe", shell)
	} else {
		require.Equal(t, "/bin/sh", shell)
	}

	// The prompt is wired so the marker frames the prompt.
	env := ei.Environ()
	if runtime.GOOS == "windows" {
		// cmd.exe renders PROMPT; $_ expands to CRLF.
		require.Contains(t, env, "PROMPT="+agenttest.PromptMarker+"$_")
	} else {
		// HOME is exported so the login shell sources our .profile.
		require.Contains(t, env, "HOME="+home)
		// dash does not interpret \n in PS1, so the marker is framed by
		// real newline bytes.
		b, err := os.ReadFile(filepath.Join(home, ".profile"))
		require.NoError(t, err)
		require.Contains(t, string(b), "\n"+agenttest.PromptMarker+"\n")
	}
}

// TestShellEnvInfo_WithHomeDir asserts the HOME override is honored.
func TestShellEnvInfo_WithHomeDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ei := agenttest.ShellEnvInfo(t, agenttest.WithHomeDir(dir))
	home, err := ei.HomeDir()
	require.NoError(t, err)
	require.Equal(t, dir, home)
}

// TestShellEnvInfo_WithShellBash asserts a named shell is resolved and that
// bash gets a .bash_profile, which it reads in preference to .profile.
func TestShellEnvInfo_WithShellBash(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("bash login profile is POSIX")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}

	ei := agenttest.ShellEnvInfo(t, agenttest.WithShell("bash"))
	shell, err := ei.Shell("")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(shell, "bash"), "shell %q should be bash", shell)

	home, err := ei.HomeDir()
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(home, ".bash_profile"))
	require.NoError(t, err)
	require.Contains(t, string(b), agenttest.PromptMarker)
}
