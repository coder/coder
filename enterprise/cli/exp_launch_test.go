package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeClaude puts a stub `claude` on PATH that dumps its environment and args
// to a file, then exits with $EXIT. It returns the dump's path.
func fakeClaude(t *testing.T) string {
	t.Helper()

	bin := t.TempDir()
	script := "#!/bin/sh\nenv > \"$OUT\"\necho \"$@\" >> \"$OUT\"\nexit \"${EXIT:-0}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)) //nolint:gosec

	out := filepath.Join(t.TempDir(), "dump")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OUT", out)
	return out
}

// TestExpLaunch exercises `coder exp launch` against a stub client binary. It
// cannot be parallel because it uses t.Setenv.
//
//nolint:paralleltest,tparallel
func TestExpLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("The stub client is a /bin/sh script.")
	}

	t.Run("LaunchesClaudeWithGatewayEnvAndPassthroughArgs", func(t *testing.T) {
		// Given: inherited vars that would re-route Claude Code off the gateway.
		t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
		t.Setenv("ANTHROPIC_API_KEY", "stale")
		out := fakeClaude(t)

		inv, _ := newCLI(t, "exp", "launch", "claude",
			"--url", "https://c.example.com", "--token", "tok",
			"--provider", "bedrock", "--model", "m",
			"--", "--dangerously-skip-permissions")

		// When: the client is launched.
		require.NoError(t, inv.Run())

		// Then: it is pointed at the deployment's gateway with the Coder
		// session token, the stale vars are gone, and the args pass through.
		dump, err := os.ReadFile(out)
		require.NoError(t, err)
		lines := strings.Split(strings.TrimRight(string(dump), "\n"), "\n")
		require.Contains(t, lines, "ANTHROPIC_BASE_URL=https://c.example.com/api/v2/ai-gateway/bedrock")
		require.Contains(t, lines, "ANTHROPIC_AUTH_TOKEN=tok")
		require.NotContains(t, lines, "CLAUDE_CODE_USE_BEDROCK=1")
		require.NotContains(t, lines, "ANTHROPIC_API_KEY=stale")
		require.Equal(t, "--model m --dangerously-skip-permissions", lines[len(lines)-1])
	})

	t.Run("PrefersTheGatewayURLFlag", func(t *testing.T) {
		out := fakeClaude(t)

		inv, _ := newCLI(t, "exp", "launch", "claude",
			"--url", "https://c.example.com", "--token", "tok",
			"--gateway-url", "https://gw.example.com/")

		// When: an explicit gateway is given with a trailing slash.
		require.NoError(t, inv.Run())

		// Then: it replaces the derived URL, without a doubled slash.
		dump, err := os.ReadFile(out)
		require.NoError(t, err)
		require.Contains(t, strings.Split(string(dump), "\n"), "ANTHROPIC_BASE_URL=https://gw.example.com/anthropic")
	})

	t.Run("RejectsAnUnknownClient", func(t *testing.T) {
		fakeClaude(t)

		inv, _ := newCLI(t, "exp", "launch", "nope",
			"--url", "https://c.example.com", "--token", "tok")

		// When: a client we do not support is named.
		err := inv.Run()

		// Then: the error names it and lists what is supported.
		require.Error(t, err)
		require.Contains(t, err.Error(), `unknown client "nope"`)
		require.Contains(t, err.Error(), "claude")
	})

	t.Run("SaysWhenTheClientIsNotInstalled", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		inv, _ := newCLI(t, "exp", "launch", "claude",
			"--url", "https://c.example.com", "--token", "tok")

		// When: claude is not on PATH. Then: the error says how to install it.
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "claude not found in PATH")
	})

	t.Run("RequiresALogin", func(t *testing.T) {
		fakeClaude(t)

		inv, _ := newCLI(t, "exp", "launch", "claude", "--url", "https://c.example.com")

		// When: there is no session token. Then: the error says how to get one.
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "coder login")
	})

	t.Run("PropagatesTheClientExitCode", func(t *testing.T) {
		fakeClaude(t)
		t.Setenv("EXIT", "3")

		inv, _ := newCLI(t, "exp", "launch", "claude",
			"--url", "https://c.example.com", "--token", "tok")

		// When: the client exits non-zero.
		err := inv.Run()

		// Then: the CLI exits with the same code.
		require.Error(t, err)
		var exitErr interface{ ExitCode() int }
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 3, exitErr.ExitCode())
	})
}
