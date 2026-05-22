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

func TestHasSuspiciousTrailingNewline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		suspicious bool
	}{
		{name: "NoTrailingNewline", input: "token", suspicious: false},
		{name: "SingleTrailingLF", input: "token\n", suspicious: true},
		{name: "SingleTrailingCRLF", input: "token\r\n", suspicious: true},
		{name: "SingleTrailingCR", input: "token\r", suspicious: true},
		{name: "MultilineValue", input: "line1\nline2\n", suspicious: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.suspicious, hasSuspiciousTrailingNewline(tt.input))
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

func TestTrailingNewlineWarnings(t *testing.T) {
	t.Parallel()

	t.Run("WarnSuspiciousValue", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		warnSuspiciousTrailingNewline(&stderr, "token\n")
		require.Contains(t, stderr.String(), "secret value from stdin ends with a trailing newline")
	})

	t.Run("DoesNotWarnForMultiline", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		warnSuspiciousTrailingNewline(&stderr, "line1\nline2\n")
		require.Empty(t, stderr.String())
	})

	t.Run("SecretValueWarnsAndPreservesValue", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		inv := newSecretTestInvocation(t, strings.NewReader("token\n"), &stderr)

		got, ok, err := secretValue(inv, "")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "token\n", got)
		require.Contains(t, stderr.String(), "secret value from stdin ends with a trailing newline")
	})

	t.Run("SecretValueDoesNotWarnForMultiline", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer
		inv := newSecretTestInvocation(t, strings.NewReader("line1\nline2\n"), &stderr)

		got, ok, err := secretValue(inv, "")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "line1\nline2\n", got)
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
		Stdin:   stdin,
		Stderr:  stderr,
		Command: &serpent.Command{},
		Args:    []string{"api-key"},
	}).WithTestParsedFlags(t, flags)
	return inv
}
