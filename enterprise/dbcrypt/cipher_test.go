package dbcrypt_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/dbcrypt"
)

func TestCipherAES256(t *testing.T) {
	t.Parallel()

	t.Run("ValidInput", func(t *testing.T) {
		t.Parallel()
		key := bytes.Repeat([]byte{'a'}, 32)
		cipher, err := dbcrypt.CipherAES256(key)
		require.NoError(t, err)

		output, err := cipher.Encrypt([]byte("hello world"))
		require.NoError(t, err)

		response, err := cipher.Decrypt(output)
		require.NoError(t, err)
		require.Equal(t, "hello world", string(response))
	})

	t.Run("InvalidInput", func(t *testing.T) {
		t.Parallel()
		key := bytes.Repeat([]byte{'a'}, 32)
		cipher, err := dbcrypt.CipherAES256(key)
		require.NoError(t, err)
		_, err = cipher.Decrypt(bytes.Repeat([]byte{'a'}, 100))
		var decryptErr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &decryptErr)
	})

	t.Run("InvalidKeySize", func(t *testing.T) {
		t.Parallel()

		_, err := dbcrypt.CipherAES256(bytes.Repeat([]byte{'a'}, 31))
		require.ErrorContains(t, err, "key must be 32 bytes")
	})
}
