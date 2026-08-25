package oauth2provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2AuthorizationServerMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	serverURL := client.URL

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Use a plain HTTP client since this endpoint doesn't require authentication.
	// Add a short readiness wait to avoid rare races with server startup.
	endpoint := serverURL.ResolveReference(&url.URL{Path: "/.well-known/oauth-authorization-server"}).String()
	var metadata codersdk.OAuth2AuthorizationServerMetadata
	testutil.RequireEventuallyResponseOK(ctx, t, endpoint, &metadata)

	// Verify the metadata
	require.NotEmpty(t, metadata.Issuer)
	require.NotEmpty(t, metadata.AuthorizationEndpoint)
	require.NotEmpty(t, metadata.TokenEndpoint)
	require.Contains(t, metadata.ResponseTypesSupported, codersdk.OAuth2ProviderResponseTypeCode)
	require.Contains(t, metadata.GrantTypesSupported, codersdk.OAuth2ProviderGrantTypeAuthorizationCode)
	require.Contains(t, metadata.GrantTypesSupported, codersdk.OAuth2ProviderGrantTypeRefreshToken)
	require.Contains(t, metadata.CodeChallengeMethodsSupported, codersdk.OAuth2PKCECodeChallengeMethodS256)
	// Pins the exact advertised set, not just that it contains something
	// expected: a hardcoded list that dropped an accepted method or kept an
	// unhonored one ("none": the token endpoint doesn't accept it yet) would
	// still pass a Contains-only check.
	require.ElementsMatch(t, codersdk.AdvertisedOAuth2TokenEndpointAuthMethods(), metadata.TokenEndpointAuthMethodsSupported)
	// Supported scopes are published from the curated catalog
	require.Equal(t, rbac.ExternalScopeNames(), metadata.ScopesSupported)
}

// TestGetAuthorizationServerMetadata_DCREnabled is a focused unit test on
// the discovery handler itself, bypassing the full coderdtest HTTP server.
// It verifies the dynamic-client-registration-enabled gate: registration_endpoint
// is advertised once an admin explicitly enables DCR, omitted when explicitly
// disabled, and omitted by default when the setting has never been
// configured.
func TestGetAuthorizationServerMetadata_DCREnabled(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://oauth2-metadata-dcr-test.example.com")
	require.NoError(t, err)

	tests := []struct {
		name string
		// configureDCR is nil for "never configured".
		configureDCR             *bool
		wantRegistrationEndpoint bool
	}{
		{
			name:                     "EnabledAdvertisesRegistrationEndpoint",
			configureDCR:             new(true),
			wantRegistrationEndpoint: true,
		},
		{
			name:                     "DisabledOmitsRegistrationEndpoint",
			configureDCR:             new(false),
			wantRegistrationEndpoint: false,
		},
		{
			name:                     "NeverConfiguredDefaultsToOmitted",
			configureDCR:             nil,
			wantRegistrationEndpoint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			db, _ := dbtestutil.NewDB(t)
			if tt.configureDCR != nil {
				err := db.UpsertOAuth2DCREnabled(ctx, *tt.configureDCR)
				require.NoError(t, err)
			}

			handler := oauth2provider.GetAuthorizationServerMetadata(db, accessURL)

			r := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil).WithContext(ctx)
			rw := httptest.NewRecorder()

			handler.ServeHTTP(rw, r)
			require.Equal(t, http.StatusOK, rw.Code)

			var metadata codersdk.OAuth2AuthorizationServerMetadata
			require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &metadata))

			if tt.wantRegistrationEndpoint {
				require.NotEmpty(t, metadata.RegistrationEndpoint)
			} else {
				require.Empty(t, metadata.RegistrationEndpoint)
			}
		})
	}
}

func TestOAuth2ProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	serverURL := client.URL

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Use a plain HTTP client since this endpoint doesn't require authentication.
	// Add a short readiness wait to avoid rare races with server startup.
	endpoint := serverURL.ResolveReference(&url.URL{Path: "/.well-known/oauth-protected-resource"}).String()
	var metadata codersdk.OAuth2ProtectedResourceMetadata
	testutil.RequireEventuallyResponseOK(ctx, t, endpoint, &metadata)

	// Verify the metadata
	require.NotEmpty(t, metadata.Resource)
	require.NotEmpty(t, metadata.AuthorizationServers)
	require.Len(t, metadata.AuthorizationServers, 1)
	require.Equal(t, metadata.Resource, metadata.AuthorizationServers[0])
	// RFC 6750 bearer tokens are now supported as fallback methods
	require.Contains(t, metadata.BearerMethodsSupported, "header")
	require.Contains(t, metadata.BearerMethodsSupported, "query")
	// Supported scopes are published from the curated catalog
	require.Equal(t, rbac.ExternalScopeNames(), metadata.ScopesSupported)
}
