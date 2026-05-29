package aigatewaykey_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aigatewaykey"
	"github.com/coder/coder/v2/coderd/apikey"
)

func TestNew(t *testing.T) {
	t.Parallel()

	params, key, err := aigatewaykey.New("test-key")
	require.NoError(t, err)
	require.Len(t, key, aigatewaykey.KeyLength)
	require.Len(t, params.SecretPrefix, aigatewaykey.KeyPrefixLength)
	require.Equal(t, key[:aigatewaykey.KeyPrefixLength], params.SecretPrefix)
	require.True(t, apikey.ValidateHash(params.HashedSecret, key))
	require.False(t, apikey.ValidateHash(params.HashedSecret, key[aigatewaykey.KeyPrefixLength:]))
}
