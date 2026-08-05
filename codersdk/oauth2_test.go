package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestOAuth2ClientRegistrationRequest_DetermineClientType verifies that the
// client type is derived from the requested token_endpoint_auth_method
// (RFC 7591 §2, OAuth 2.1 §2.1), not hardcoded to "confidential".
func TestOAuth2ClientRegistrationRequest_DetermineClientType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// authMethod is the requested token_endpoint_auth_method, applied
		// before ApplyDefaults if applyDefaults is set.
		authMethod codersdk.OAuth2TokenEndpointAuthMethod
		// applyDefaults runs ApplyDefaults() before DetermineClientType(),
		// matching the real request path where an omitted auth method is
		// defaulted to "client_secret_basic" before this check ever runs.
		applyDefaults bool
		expectedType  string
	}{
		{
			name:         "NoneIsPublic",
			authMethod:   codersdk.OAuth2TokenEndpointAuthMethodNone,
			expectedType: "public",
		},
		{
			name:         "ClientSecretBasicIsConfidential",
			authMethod:   codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			expectedType: "confidential",
		},
		{
			name:         "ClientSecretPostIsConfidential",
			authMethod:   codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
			expectedType: "confidential",
		},
		{
			name:          "OmittedDefaultsToConfidentialAfterApplyDefaults",
			applyDefaults: true,
			expectedType:  "confidential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := codersdk.OAuth2ClientRegistrationRequest{
				TokenEndpointAuthMethod: tt.authMethod,
			}
			if tt.applyDefaults {
				req = req.ApplyDefaults()
				require.Equal(t, codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic, req.TokenEndpointAuthMethod)
			}
			require.Equal(t, tt.expectedType, req.DetermineClientType())
		})
	}
}
