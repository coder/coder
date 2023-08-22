package cryptorand_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

func TestCipherAES256(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{'a'}, 32)
	cipher, err := cryptorand.CipherAES256(key)
	require.NoError(t, err)

	output, err := cipher.Encrypt([]byte("hello world"))
	require.NoError(t, err)

	response, err := cipher.Decrypt(output)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(response))

	t.Run("InvalidInput", func(t *testing.T) {
		t.Parallel()
		_, err := cipher.Decrypt(bytes.Repeat([]byte{'a'}, 100))
		require.NoError(t, err)
	})
}
