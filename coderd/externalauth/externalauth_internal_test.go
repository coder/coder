package externalauth

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestGitlabDefaults(t *testing.T) {
	t.Parallel()

	// The default cloud setup. Copying this here as hard coded
	// values.
	cloud := codersdk.ExternalAuthConfig{
		Type:        string(codersdk.EnhancedExternalAuthProviderGitLab),
		ID:          string(codersdk.EnhancedExternalAuthProviderGitLab),
		AuthURL:     "https://gitlab.com/oauth/authorize",
		TokenURL:    "https://gitlab.com/oauth/token",
		ValidateURL: "https://gitlab.com/oauth/token/info",
		DisplayName: "GitLab",
		DisplayIcon: "/icon/gitlab.svg",
		Regex:       `^(https?://)?gitlab\.com(/.*)?$`,
		Scopes:      []string{"write_repository"},
	}

	tests := []struct {
		name           string
		input          codersdk.ExternalAuthConfig
		expected       codersdk.ExternalAuthConfig
		mutateExpected func(*codersdk.ExternalAuthConfig)
	}{
		// Cloud
		{
			name: "OnlyType",
			input: codersdk.ExternalAuthConfig{
				Type: string(codersdk.EnhancedExternalAuthProviderGitLab),
			},
			expected: cloud,
		},
		{
			// If someone was to manually configure the gitlab cli.
			name: "CloudByConfig",
			input: codersdk.ExternalAuthConfig{
				Type:    string(codersdk.EnhancedExternalAuthProviderGitLab),
				AuthURL: "https://gitlab.com/oauth/authorize",
			},
			expected: cloud,
		},
		{
			// Changing some of the defaults of the cloud option
			name: "CloudWithChanges",
			input: codersdk.ExternalAuthConfig{
				Type: string(codersdk.EnhancedExternalAuthProviderGitLab),
				// Adding an extra query param intentionally to break simple
				// string comparisons.
				AuthURL:     "https://gitlab.com/oauth/authorize?foo=bar",
				DisplayName: "custom",
				Regex:       ".*",
			},
			expected: cloud,
			mutateExpected: func(config *codersdk.ExternalAuthConfig) {
				config.AuthURL = "https://gitlab.com/oauth/authorize?foo=bar"
				config.DisplayName = "custom"
				config.Regex = ".*"
			},
		},
		// Self-hosted
		{
			// Dynamically figures out the Validate, Token, and Regex fields.
			name: "SelfHostedOnlyAuthURL",
			input: codersdk.ExternalAuthConfig{
				Type:    string(codersdk.EnhancedExternalAuthProviderGitLab),
				AuthURL: "https://gitlab.company.org/oauth/authorize?foo=bar",
			},
			expected: cloud,
			mutateExpected: func(config *codersdk.ExternalAuthConfig) {
				config.AuthURL = "https://gitlab.company.org/oauth/authorize?foo=bar"
				config.ValidateURL = "https://gitlab.company.org/oauth/token/info"
				config.TokenURL = "https://gitlab.company.org/oauth/token"
				config.Regex = `^(https?://)?gitlab\.company\.org(/.*)?$`
			},
		},
		{
			// Strange values
			name: "RandomValues",
			input: codersdk.ExternalAuthConfig{
				Type:        string(codersdk.EnhancedExternalAuthProviderGitLab),
				AuthURL:     "https://auth.com/auth",
				ValidateURL: "https://validate.com/validate",
				TokenURL:    "https://token.com/token",
				Regex:       "random",
			},
			expected: cloud,
			mutateExpected: func(config *codersdk.ExternalAuthConfig) {
				config.AuthURL = "https://auth.com/auth"
				config.ValidateURL = "https://validate.com/validate"
				config.TokenURL = "https://token.com/token"
				config.Regex = `random`
			},
		},
	}
	for _, c := range tests {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			applyDefaultsToConfig(&c.input)
			if c.mutateExpected != nil {
				c.mutateExpected(&c.expected)
			}
			require.Equal(t, c.input, c.expected)
		})
	}
}

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
