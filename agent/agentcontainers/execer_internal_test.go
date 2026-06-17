package agentcontainers

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/testutil"
)

func TestCommandEnvExecer_Prepare(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the POSIX shell quoting under test does not apply on Windows")
	}

	const shell = "/bin/sh"
	commandEnv := func(usershell.EnvInfoer, []string) (string, string, []string, error) {
		return shell, "/tmp", []string{"FOO=bar"}, nil
	}
	e := newCommandEnvExecer(slogtest.Make(t, nil).Leveled(slog.LevelDebug), commandEnv, agentexec.DefaultExecer)

	t.Run("ArgvPassthrough", func(t *testing.T) {
		t.Parallel()

		name, args, dir, env := e.prepare(context.Background(), "echo", "hello", "world")
		// The command is run as: shell -c "$@" "" <argv...> so that the
		// shell re-emits argv without re-parsing it. The empty $0 slot is
		// discarded.
		require.Equal(t, shell, name)
		require.Equal(t, []string{"-c", `"$@"`, "", "echo", "hello", "world"}, args)
		require.Equal(t, "/tmp", dir)
		require.Equal(t, []string{"FOO=bar"}, env)
	})

	t.Run("MetacharactersNotInterpreted", func(t *testing.T) {
		t.Parallel()

		payloads := []string{
			"$(echo INJECTED)",
			"`echo INJECTED`",
			"$HOME",
			"a; echo INJECTED",
			"a && echo INJECTED",
			"a | echo INJECTED",
			"a\necho INJECTED",
			"it's a \"test\" \\ end",
			"",
		}
		for _, payload := range payloads {
			ctx := testutil.Context(t, testutil.WaitShort)
			cmd := e.CommandContext(ctx, "printf", "%s", payload)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			require.NoError(t, cmd.Run(), "payload %q", payload)
			assert.Equal(t, payload, out.String(), "payload %q was altered by the shell", payload)
		}
	})

	t.Run("CommandSubstitutionHasNoSideEffect", func(t *testing.T) {
		t.Parallel()

		marker := filepath.Join(t.TempDir(), "pwned")
		ctx := testutil.Context(t, testutil.WaitShort)
		cmd := e.CommandContext(ctx, "echo", "$(touch "+marker+")")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		require.NoError(t, cmd.Run())
		require.Equal(t, "$(touch "+marker+")\n", out.String())
		require.NoFileExists(t, marker, "command substitution executed; injection is possible")
	})
}
