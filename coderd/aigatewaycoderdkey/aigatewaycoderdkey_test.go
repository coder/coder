package aigatewaycoderdkey_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aigatewaycoderdkey"
	"github.com/coder/coder/v2/coderd/apikey"
)

func TestNew(t *testing.T) {
	t.Parallel()

	params, key, err := aigatewaycoderdkey.New("test-key")
	require.NoError(t, err)
	require.Len(t, key, aigatewaycoderdkey.KeyLength)
	require.Len(t, params.SecretPrefix, aigatewaycoderdkey.KeyPrefixLength)
	require.Equal(t, key[:aigatewaycoderdkey.KeyPrefixLength], params.SecretPrefix)
	require.True(t, apikey.ValidateHash(params.HashedSecret, key))
	require.False(t, apikey.ValidateHash(params.HashedSecret, key[aigatewaycoderdkey.KeyPrefixLength:]))
}
