//go:build !slim

package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
func TestRotateFlagsApplyDeprecatedEnv(t *testing.T) {
	t.Run("NewKey", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_NEW_KEY", "test-key")

		var f rotateFlags
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, "test-key", f.New)
		require.Contains(t, buf.String(), "CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_NEW_KEY is deprecated")
	})

	t.Run("NewKeyNotOverridden", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_NEW_KEY", "old-value")

		f := rotateFlags{New: "new-value"}
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, "new-value", f.New)
		require.Empty(t, buf.String())
	})

	t.Run("OldKeys", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_OLD_KEYS", "key1,key2")

		var f rotateFlags
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, []string{"key1", "key2"}, f.Old)
		require.Contains(t, buf.String(), "CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_OLD_KEYS is deprecated")
	})

	t.Run("OldKeysNotOverridden", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_ENCRYPT_OLD_KEYS", "old-value")

		f := rotateFlags{Old: []string{"existing"}}
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, []string{"existing"}, f.Old)
		require.Empty(t, buf.String())
	})
}

// nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
func TestDecryptFlagsApplyDeprecatedEnv(t *testing.T) {
	t.Run("Keys", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS", "key1,key2")

		var f decryptFlags
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, []string{"key1", "key2"}, f.Keys)
		require.Contains(t, buf.String(), "CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS is deprecated")
	})

	t.Run("KeysNotOverridden", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_DECRYPT_KEYS", "old-value")

		f := decryptFlags{Keys: []string{"existing"}}
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, []string{"existing"}, f.Keys)
		require.Empty(t, buf.String())
	})
}

// nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
func TestDeleteFlagsApplyDeprecatedEnv(t *testing.T) {
	t.Run("PostgresURL", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_POSTGRES_URL", "postgres://old-url")

		var f deleteFlags
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, "postgres://old-url", f.PostgresURL)
		require.Contains(t, buf.String(), "CODER_EXTERNAL_TOKEN_ENCRYPTION_POSTGRES_URL is deprecated")
	})

	t.Run("PostgresURLNotOverridden", func(t *testing.T) {
		t.Setenv("CODER_EXTERNAL_TOKEN_ENCRYPTION_POSTGRES_URL", "postgres://old-url")

		f := deleteFlags{PostgresURL: "postgres://new-url"}
		var buf bytes.Buffer
		f.applyDeprecatedEnv(&buf)

		require.Equal(t, "postgres://new-url", f.PostgresURL)
		require.Empty(t, buf.String())
	})
}
