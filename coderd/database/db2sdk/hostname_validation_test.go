package db2sdk

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidateAppHostnameLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subdomainName string
		expected     bool
	}{
		{
			name:         "empty hostname",
			subdomainName: "",
			expected:     true,
		},
		{
			name:         "valid short hostname",
			subdomainName: "app--agent--workspace--user",
			expected:     true,
		},
		{
			name:         "valid hostname with max length segment",
			subdomainName: "a12345678901234567890123456789012345678901234567890123456789012--agent--workspace--user", // 63 chars in first segment
			expected:     true,
		},
		{
			name:         "invalid hostname with long app name",
			subdomainName: "toolongappnamethatexceedsthednslimitof63charactersforsureandshouldfail--agent--workspace--user", // 78 chars in first segment
			expected:     false,
		},
		{
			name:         "invalid hostname with long agent name",
			subdomainName: "app--toolongagentnamethatexceedsthednslimitof63charactersforsureandshouldfail--workspace--user", // 72 chars in agent segment
			expected:     false,
		},
		{
			name:         "invalid hostname with long workspace name",
			subdomainName: "app--agent--toolongworkspacenamethatexceedsthednslimitof63charactersforsureandshouldfail--user", // 77 chars in workspace segment
			expected:     false,
		},
		{
			name:         "invalid hostname with long username",
			subdomainName: "app--agent--workspace--toolongusernamethatexceedsthednslimitof63charactersforsureandshouldfail", // 72 chars in username segment
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateAppHostnameLength(tt.subdomainName)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestAppsWithHostnameLengthValidation(t *testing.T) {
	t.Parallel()

	agent := database.WorkspaceAgent{
		ID:   uuid.New(),
		Name: "agent",
	}
	workspace := database.Workspace{
		ID:   uuid.New(),
		Name: "workspace",
	}
	ownerName := "user"

	tests := []struct {
		name           string
		appSlug        string
		subdomain      bool
		expectedHealth codersdk.WorkspaceAppHealth
	}{
		{
			name:           "non-subdomain app should not be affected",
			appSlug:        "toolongappnamethatexceedsthednslimitof63charactersforsureandshouldfail",
			subdomain:      false,
			expectedHealth: codersdk.WorkspaceAppHealthHealthy,
		},
		{
			name:           "short subdomain app should remain healthy",
			appSlug:        "app",
			subdomain:      true,
			expectedHealth: codersdk.WorkspaceAppHealthHealthy,
		},
		{
			name:           "long subdomain app should become unhealthy",
			appSlug:        "toolongappnamethatexceedsthednslimitof63charactersforsureandshouldfail",
			subdomain:      true,
			expectedHealth: codersdk.WorkspaceAppHealthUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbApps := []database.WorkspaceApp{
				{
					ID:          uuid.New(),
					Slug:        tt.appSlug,
					DisplayName: "Test App",
					Subdomain:   tt.subdomain,
					Health:      database.WorkspaceAppHealthHealthy, // Start as healthy
				},
			}

			apps := Apps(dbApps, []database.WorkspaceAppStatus{}, agent, ownerName, workspace)
			require.Len(t, apps, 1)
			require.Equal(t, tt.expectedHealth, apps[0].Health)
		})
	}
}