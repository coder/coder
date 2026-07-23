package keys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/keys"
	"github.com/coder/coder/v2/coderd/apikey"
)

func TestNew(t *testing.T) {
	t.Parallel()

	params, key, err := keys.New("test-key")
	require.NoError(t, err)
	require.Len(t, key, keys.KeyLength)
	require.Len(t, params.SecretPrefix, keys.KeyPrefixLength)
	require.Equal(t, key[:keys.KeyPrefixLength], params.SecretPrefix)
	require.True(t, apikey.ValidateHash(params.HashedSecret, key))
	require.False(t, apikey.ValidateHash(params.HashedSecret, key[keys.KeyPrefixLength:]))
}
