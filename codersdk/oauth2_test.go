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
		name       string
		authMethod codersdk.OAuth2TokenEndpointAuthMethod
		// applyDefaults runs ApplyDefaults() before DetermineClientType(),
		// matching the real request path where an omitted auth method is
		// defaulted to "client_secret_basic" before this check ever runs.
		applyDefaults bool
		// wantAuthMethodAfterDefaults pins what ApplyDefaults() does to
		// authMethod, so a case that runs applyDefaults also verifies
		// ApplyDefaults left (or changed) the field as expected before
		// DetermineClientType() reads it. Only checked when applyDefaults
		// is true.
		wantAuthMethodAfterDefaults codersdk.OAuth2TokenEndpointAuthMethod
		expectedType                string
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
			// ApplyDefaults only fills an empty auth method; it must not
			// touch an explicit "none". If it ever grew a rule that did,
			// the pre-defaults Validate() call and the post-defaults
			// storage call would disagree about this client's type.
			name:                        "NoneStaysPublicAfterApplyDefaults",
			authMethod:                  codersdk.OAuth2TokenEndpointAuthMethodNone,
			applyDefaults:               true,
			wantAuthMethodAfterDefaults: codersdk.OAuth2TokenEndpointAuthMethodNone,
			expectedType:                "public",
		},
		{
			// An omitted auth method must not be read as public. Without
			// ApplyDefaults the empty string also falls through to
			// confidential, so this is safe in either order, but the real
			// path always defaults first.
			name:                        "OmittedDefaultsToConfidentialAfterApplyDefaults",
			applyDefaults:               true,
			wantAuthMethodAfterDefaults: codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
			expectedType:                "confidential",
		},
		{
			name:         "OmittedIsConfidentialWithoutApplyDefaults",
			expectedType: "confidential",
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
				require.Equal(t, tt.wantAuthMethodAfterDefaults, req.TokenEndpointAuthMethod)
			}
			require.Equal(t, tt.expectedType, string(req.DetermineClientType()))
		})
	}
}
