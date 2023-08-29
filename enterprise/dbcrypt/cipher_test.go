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

func TestCiphersAES256(t *testing.T) {
	t.Parallel()

	// Given: two ciphers
	key1 := bytes.Repeat([]byte{'a'}, 32)
	key2 := bytes.Repeat([]byte{'b'}, 32)
	cipher1, err := dbcrypt.CipherAES256(key1)
	require.NoError(t, err)
	cipher2, err := dbcrypt.CipherAES256(key2)
	require.NoError(t, err)

	ciphers := dbcrypt.CiphersAES256(
		cipher1,
		cipher2,
	)

	// Then: it should encrypt with the cipher1
	output, err := ciphers.Encrypt([]byte("hello world"))
	require.NoError(t, err)
	// The first 7 bytes of the output should be the hex digest of cipher1
	require.Equal(t, cipher1.HexDigest(), string(output[:7]))

	// And: it should decrypt successfully
	decrypted, err := ciphers.Decrypt(output)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(decrypted))

	// Decryption of the above should fail with cipher2
	_, err = cipher2.Decrypt(output)
	var decryptErr *dbcrypt.DecryptFailedError
	require.ErrorAs(t, err, &decryptErr)

	// Decryption of data encrypted with cipher2 should succeed
	output2, err := cipher2.Encrypt([]byte("hello world"))
	require.NoError(t, err)
	decrypted2, err := ciphers.Decrypt(bytes.Join([][]byte{[]byte(cipher2.HexDigest()), output2}, []byte{'-'}))
	require.NoError(t, err)
	require.Equal(t, "hello world", string(decrypted2))
}
