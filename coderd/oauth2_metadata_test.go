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

func TestOAuth2ProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Get the protected resource metadata
	resp, err := client.Request(ctx, http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var metadata codersdk.OAuth2ProtectedResourceMetadata
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	require.NoError(t, err)

	// Verify the metadata
	require.NotEmpty(t, metadata.Resource)
	require.NotEmpty(t, metadata.AuthorizationServers)
	require.Len(t, metadata.AuthorizationServers, 1)
	require.Equal(t, metadata.Resource, metadata.AuthorizationServers[0])
	// BearerMethodsSupported is omitted since Coder uses custom authentication methods
	// Standard RFC 6750 bearer tokens are not supported
	require.True(t, len(metadata.BearerMethodsSupported) == 0)
	// ScopesSupported can be empty until scope system is implemented
	// Empty slice is marshaled as empty array, but can be nil when unmarshaled
	require.True(t, len(metadata.ScopesSupported) == 0)
}
