package oauth2provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2AuthorizationServerMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	serverURL := client.URL

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Use a plain HTTP client since this endpoint doesn't require authentication
	endpoint := serverURL.ResolveReference(&url.URL{Path: "/.well-known/oauth-authorization-server"}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
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
	serverURL := client.URL

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Use a plain HTTP client since this endpoint doesn't require authentication
	endpoint := serverURL.ResolveReference(&url.URL{Path: "/.well-known/oauth-protected-resource"}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
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
	// RFC 6750 bearer tokens are now supported as fallback methods
	require.Contains(t, metadata.BearerMethodsSupported, "header")
	require.Contains(t, metadata.BearerMethodsSupported, "query")
	// ScopesSupported can be empty until scope system is implemented
	// Empty slice is marshaled as empty array, but can be nil when unmarshaled
	require.True(t, len(metadata.ScopesSupported) == 0)
}
