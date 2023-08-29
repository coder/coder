package dbcrypt_test

import (
	"bytes"
	"encoding/base64"
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

	t.Run("TestNonce", func(t *testing.T) {
		t.Parallel()
		key := bytes.Repeat([]byte{'a'}, 32)
		cipher, err := dbcrypt.CipherAES256(key)
		require.NoError(t, err)
		require.Equal(t, "3ba3f5f", cipher.HexDigest())

		encrypted1, err := cipher.Encrypt([]byte("hello world"))
		require.NoError(t, err)
		encrypted2, err := cipher.Encrypt([]byte("hello world"))
		require.NoError(t, err)
		require.NotEqual(t, encrypted1, encrypted2, "nonce should be different for each encryption")

		munged := make([]byte, len(encrypted1))
		copy(munged, encrypted1)
		munged[0] = munged[0] ^ 0xff
		_, err = cipher.Decrypt(munged)
		var decryptErr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &decryptErr, "munging the first byte of the encrypted data should cause decryption to fail")
	})
}

func TestCiphers(t *testing.T) {
	t.Parallel()

	// Given: two ciphers
	key1 := bytes.Repeat([]byte{'a'}, 32)
	key2 := bytes.Repeat([]byte{'b'}, 32)
	cipher1, err := dbcrypt.CipherAES256(key1)
	require.NoError(t, err)
	cipher2, err := dbcrypt.CipherAES256(key2)
	require.NoError(t, err)

	ciphers := dbcrypt.NewCiphers(cipher1, cipher2)

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

	// Decryption of data encrypted with cipher1 should succeed
	output1, err := cipher1.Encrypt([]byte("hello world"))
	require.NoError(t, err)
	decrypted1, err := ciphers.Decrypt(bytes.Join([][]byte{[]byte(cipher1.HexDigest()), output1}, []byte{'-'}))
	require.NoError(t, err)
	require.Equal(t, "hello world", string(decrypted1))

	// Wrapping a Ciphers with itself should panic.
	require.PanicsWithValue(t, "developer error: do not nest Ciphers", func() {
		_ = dbcrypt.NewCiphers(ciphers)
	})
}

// This test ensures backwards compatibility. If it breaks, something is very wrong.
func TestCiphersBackwardCompatibility(t *testing.T) {
	t.Parallel()
	var (
		msg = "hello world"
		key = bytes.Repeat([]byte{'a'}, 32)
		//nolint: gosec // The below is the base64-encoded result of encrypting the above message with the above key.
		encoded = `M2JhM2Y1Zi3r1KSStbmfMBXDzdjVcCrtumdMFsJ4QiYlb3fV1HB8yxg9obHaz5I=`
	)

	// This is the code that was used to generate the above.
	// Note that the output of this code will change every time it is run.
	//encrypted, err := cs.Encrypt([]byte(msg))
	//require.NoError(t, err)
	//t.Logf("encoded: %q", base64.StdEncoding.EncodeToString(encrypted))

	cipher, err := dbcrypt.CipherAES256(key)
	require.NoError(t, err)
	require.Equal(t, "3ba3f5f", cipher.HexDigest())
	cs := dbcrypt.NewCiphers(cipher)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err, "the encoded string should be valid base64")
	decrypted, err := cs.Decrypt(decoded)
	require.NoError(t, err, "decryption should succeed")
	require.Equal(t, msg, string(decrypted), "decrypted message should match original message")
}
