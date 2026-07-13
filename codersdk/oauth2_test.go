package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestOAuth2ClientRegistrationRequest_DetermineClientType covers D1-01: the
// client type must be derived from the requested token_endpoint_auth_method
// (RFC 7591 §2, OAuth 2.1 §2.1), not hardcoded to "confidential".
func TestOAuth2ClientRegistrationRequest_DetermineClientType(t *testing.T) {
	t.Parallel()

	t.Run("NoneIsPublic", func(t *testing.T) {
		t.Parallel()
		req := codersdk.OAuth2ClientRegistrationRequest{
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
		}
		require.Equal(t, "public", req.DetermineClientType())
	})

	t.Run("ClientSecretBasicIsConfidential", func(t *testing.T) {
		t.Parallel()
		req := codersdk.OAuth2ClientRegistrationRequest{
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
		}
		require.Equal(t, "confidential", req.DetermineClientType())
	})

	t.Run("ClientSecretPostIsConfidential", func(t *testing.T) {
		t.Parallel()
		req := codersdk.OAuth2ClientRegistrationRequest{
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
		}
		require.Equal(t, "confidential", req.DetermineClientType())
	})

	t.Run("OmittedDefaultsToConfidentialAfterApplyDefaults", func(t *testing.T) {
		t.Parallel()
		// ApplyDefaults must run first in the real request path, so an
		// omitted auth method is never misread as public.
		req := codersdk.OAuth2ClientRegistrationRequest{}.ApplyDefaults()
		require.Equal(t, codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic, req.TokenEndpointAuthMethod)
		require.Equal(t, "confidential", req.DetermineClientType())
	})
}
