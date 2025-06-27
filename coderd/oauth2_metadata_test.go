package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2AuthorizationServerMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Get the metadata
	resp, err := client.Request(ctx, http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var metadata codersdk.OAuth2AuthorizationServerMetadata
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	require.NoError(t, err)

	// Verify the metadata
	require.NotEmpty(t, metadata.Issuer)
	require.NotEmpty(t, metadata.AuthorizationEndpoint)
	require.NotEmpty(t, metadata.TokenEndpoint)
	require.Contains(t, metadata.ResponseTypesSupported, "code")
	require.Contains(t, metadata.GrantTypesSupported, "authorization_code")
	require.Contains(t, metadata.GrantTypesSupported, "refresh_token")
	require.Contains(t, metadata.CodeChallengeMethodsSupported, "S256")
}
