package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
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

func TestSecretsFileFormatFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want codersdk.SecretsFileFormat
	}{
		{path: ".env", want: codersdk.SecretsFileFormatEnv},
		{path: "/tmp/prod.env", want: codersdk.SecretsFileFormatEnv},
		{path: "secrets.ENV", want: codersdk.SecretsFileFormatEnv},
		{path: "config.json", want: codersdk.SecretsFileFormatJSON},
		{path: "values.yaml", want: codersdk.SecretsFileFormatYAML},
		{path: "values.yml", want: codersdk.SecretsFileFormatYAML},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			got, err := secretsFileFormatFromPath(tt.path)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	// filepath.Ext(".env.local") is ".local", so it does not map to a format.
	for _, path := range []string{"secrets.txt", "noextension", ".env.local", "-", ""} {
		t.Run("Unsupported/"+path, func(t *testing.T) {
			t.Parallel()

			_, err := secretsFileFormatFromPath(path)
			require.ErrorContains(t, err, "set --input-format to one of: env, json, yaml")
		})
	}
}

func TestReadSecretsFile(t *testing.T) {
	t.Parallel()

	t.Run("Stdin", func(t *testing.T) {
		t.Parallel()

		inv := newSecretTestInvocation(t, strings.NewReader("A=1"), nil)

		got, err := readSecretsFile(inv, "-")
		require.NoError(t, err)
		require.Equal(t, "A=1", string(got))
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "secrets.env")
		require.NoError(t, os.WriteFile(path, []byte("A=1"), 0o600))
		inv := newSecretTestInvocation(t, strings.NewReader(""), nil)

		got, err := readSecretsFile(inv, path)
		require.NoError(t, err)
		require.Equal(t, "A=1", string(got))
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()

		inv := newSecretTestInvocation(t, strings.NewReader(""), nil)

		_, err := readSecretsFile(inv, filepath.Join(t.TempDir(), "absent.env"))
		require.ErrorContains(t, err, "open secrets file")
	})

	t.Run("AtMaxSize", func(t *testing.T) {
		t.Parallel()

		content := strings.Repeat("a", codersdk.MaxSecretsFileBytes)
		inv := newSecretTestInvocation(t, strings.NewReader(content), nil)

		got, err := readSecretsFile(inv, "-")
		require.NoError(t, err)
		require.Len(t, got, codersdk.MaxSecretsFileBytes)
	})

	t.Run("OverMaxSize", func(t *testing.T) {
		t.Parallel()

		content := strings.Repeat("a", codersdk.MaxSecretsFileBytes+1)
		inv := newSecretTestInvocation(t, strings.NewReader(content), nil)

		_, err := readSecretsFile(inv, "-")
		require.ErrorContains(t, err, "exceeds the maximum allowed size")
	})
}

func TestWarnSecretsWithoutEnvNameEscapesNames(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	warnSecretsWithoutEnvName(&stderr, []codersdk.UserSecret{{Name: "\x1b[31mBAD"}})

	require.Contains(t, stderr.String(), `"\x1b[31mBAD"`)
	require.NotContains(t, stderr.String(), "\x1b[31mBAD")
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
