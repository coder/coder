package externalauth

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func Test_bitbucketServerConfigDefaults(t *testing.T) {
	t.Parallel()

	bbType := string(codersdk.EnhancedExternalAuthProviderBitBucketServer)
	tests := []struct {
		name     string
		config   *codersdk.ExternalAuthConfig
		expected codersdk.ExternalAuthConfig
	}{
		{
			// Very few fields are statically defined for Bitbucket Server.
			name: "EmptyBitbucketServer",
			config: &codersdk.ExternalAuthConfig{
				Type: bbType,
			},
			expected: codersdk.ExternalAuthConfig{
				Type:        bbType,
				ID:          bbType,
				DisplayName: "Bitbucket Server",
				Scopes:      []string{"PUBLIC_REPOS", "REPO_READ", "REPO_WRITE"},
				DisplayIcon: "/icon/bitbucket.svg",
			},
		},
		{
			// Only the AuthURL is required for defaults to work.
			name: "AuthURL",
			config: &codersdk.ExternalAuthConfig{
				Type:    bbType,
				AuthURL: "https://bitbucket.example.com/login/oauth/authorize",
			},
			expected: codersdk.ExternalAuthConfig{
				Type:        bbType,
				ID:          bbType,
				AuthURL:     "https://bitbucket.example.com/login/oauth/authorize",
				TokenURL:    "https://bitbucket.example.com/rest/oauth2/latest/token",
				ValidateURL: "https://bitbucket.example.com/rest/api/latest/inbox/pull-requests/count",
				Scopes:      []string{"PUBLIC_REPOS", "REPO_READ", "REPO_WRITE"},
				Regex:       `^(https?://)?bitbucket\.example\.com(/.*)?$`,
				DisplayName: "Bitbucket Server",
				DisplayIcon: "/icon/bitbucket.svg",
			},
		},
		{
			// Ensure backwards compatibility. The type should update to "bitbucket-cloud",
			// but the ID and other fields should remain the same.
			name: "BitbucketLegacy",
			config: &codersdk.ExternalAuthConfig{
				Type: "bitbucket",
			},
			expected: codersdk.ExternalAuthConfig{
				Type:        string(codersdk.EnhancedExternalAuthProviderBitBucketCloud),
				ID:          "bitbucket", // Legacy ID remains unchanged
				AuthURL:     "https://bitbucket.org/site/oauth2/authorize",
				TokenURL:    "https://bitbucket.org/site/oauth2/access_token",
				ValidateURL: "https://api.bitbucket.org/2.0/user",
				DisplayName: "BitBucket",
				DisplayIcon: "/icon/bitbucket.svg",
				Regex:       `^(https?://)?bitbucket\.org(/.*)?$`,
				Scopes:      []string{"account", "repository:write"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			applyDefaultsToConfig(tt.config)
			require.Equal(t, tt.expected, *tt.config)
		})
	}
}
