package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"
)

func TestTrimSingleTrailingNewline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		want       string
		suspicious bool
	}{
		{name: "NoTrailingNewline", input: "token", want: "token", suspicious: false},
		{name: "SingleTrailingLF", input: "token\n", want: "token", suspicious: true},
		{name: "SingleTrailingCRLF", input: "token\r\n", want: "token", suspicious: true},
		{name: "MultilineValue", input: "line1\nline2\n", want: "line1\nline2", suspicious: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, suspicious := trimSingleTrailingNewline(tt.input)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.suspicious, suspicious)
		})
	}
}

func TestReadInvocationStdin(t *testing.T) {
	t.Parallel()

	t.Run("ZeroBytesRead", func(t *testing.T) {
		t.Parallel()

		inv := newSecretTestInvocation(t, strings.NewReader(""), nil)

		got, provided, err := readInvocationStdin(inv)
		require.NoError(t, err)
		require.False(t, provided)
		require.Empty(t, got)
	})

	t.Run("StringRead", func(t *testing.T) {
		t.Parallel()

		inv := newSecretTestInvocation(t, strings.NewReader("token"), nil)

		got, provided, err := readInvocationStdin(inv)
		require.NoError(t, err)
		require.True(t, provided)
		require.Equal(t, "token", got)
	})
}

func TestResolveInvocationStdinValue(t *testing.T) {
	t.Parallel()

	t.Run("WarningWithoutPromptKeepsValue", func(t *testing.T) {
		t.Parallel()

		origPrompt := promptSecretTrimTrailingNewline
		t.Cleanup(func() { promptSecretTrimTrailingNewline = origPrompt })
		promptSecretTrimTrailingNewline = func(inv *serpent.Invocation) (bool, bool, error) {
			return false, false, nil
		}

		var stderr bytes.Buffer
		inv := newSecretTestInvocation(t, strings.NewReader("token\n"), &stderr)

		got, err := resolveInvocationStdinValue(inv, "token\n")
		require.NoError(t, err)
		require.Equal(t, "token\n", got)
		require.Contains(t, stderr.String(), "stdin ends with a trailing newline")
	})

	t.Run("PromptAcceptedTrimsValue", func(t *testing.T) {
		t.Parallel()

		origPrompt := promptSecretTrimTrailingNewline
		t.Cleanup(func() { promptSecretTrimTrailingNewline = origPrompt })
		promptSecretTrimTrailingNewline = func(inv *serpent.Invocation) (bool, bool, error) {
			return true, true, nil
		}

		var stderr bytes.Buffer
		inv := newSecretTestInvocation(t, strings.NewReader("token\n"), &stderr)

		got, err := resolveInvocationStdinValue(inv, "token\n")
		require.NoError(t, err)
		require.Equal(t, "token", got)
		require.Empty(t, stderr.String())
	})
}

func newSecretTestInvocation(t *testing.T, stdin io.Reader, stderr io.Writer) *serpent.Invocation {
	t.Helper()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if stderr == nil {
		stderr = io.Discard
	}
	inv := (&serpent.Invocation{
		Stdin:  stdin,
		Stderr: stderr,
	}).WithTestParsedFlags(t, flags)
	return inv
}
