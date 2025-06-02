package coderd_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// createClientWithRoleTokenLifetimes creates a test client with role-based token lifetimes configured
func createClientWithRoleTokenLifetimes(t *testing.T, roleTokenLifetimesJSON string, maxTokenDuration time.Duration) *codersdk.Client {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.Sessions.MaximumTokenDuration = serpent.Duration(maxTokenDuration)
	dv.Sessions.RoleTokenLifetimes = serpent.String(roleTokenLifetimesJSON)

	// Create the client, database, and get the API instance
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Process the role token lifetimes configuration
	// This is normally done in cli/server.go but needs to be done manually in tests
	ctx := context.Background()
	logger := slog.Make(sloghuman.Sink(io.Discard))
	// Process the configuration on the API's deployment values
	// Use system context for database operations
	//nolint:gocritic // Unit test requires system context for database operations
	err := coderd.ProcessRoleTokenLifetimesConfig(dbauthz.AsSystemRestricted(ctx), &api.DeploymentValues.Sessions, api.Database, logger)
	require.NoError(t, err)

	// Verify the configuration was processed
	t.Logf("RoleTokenLifetimes config: %s", api.DeploymentValues.Sessions.RoleTokenLifetimes.Value())
	t.Logf("MaximumTokenDuration: %v", api.DeploymentValues.Sessions.MaximumTokenDuration.Value())
	// Check what the owner role lifetime is
	ownerRole := rbac.RoleIdentifier{Name: "owner"}
	ownerLifetime := api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(ownerRole)
	t.Logf("Owner role lifetime: %v", ownerLifetime)

	// Create the first user
	_ = coderdtest.CreateFirstUser(t, client)

	return client
}

func TestRoleBasedTokenLifetimes_Integration(t *testing.T) {
	t.Parallel()

	t.Run("ServerStartupWithVariousConfigs", func(t *testing.T) {
		t.Parallel()

		// Test server starts successfully with valid configs
		testCases := []struct {
			name       string
			configJSON string
			shouldFail bool
		}{
			{
				name:       "ValidSiteWideRoles",
				configJSON: `{"owner": "168h", "user-admin": "72h"}`,
				shouldFail: false,
			},
			{
				name:       "InvalidJSON",
				configJSON: `{"owner": "168h"`,
				shouldFail: true,
			},
			{
				name:       "InvalidDuration",
				configJSON: `{"owner": "invalid"}`,
				shouldFail: false, // Server should start but skip invalid entries
			},
			{
				name:       "EmptyConfig",
				configJSON: `{}`,
				shouldFail: false,
			},
			{
				name:       "MixedValidInvalid",
				configJSON: `{"owner": "168h", "user-admin": "invalid", "member": "24h"}`,
				shouldFail: false, // Server should start, skip invalid
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dv := coderdtest.DeploymentValues(t)
				dv.Sessions.RoleTokenLifetimes = serpent.String(tc.configJSON)

				if tc.shouldFail {
					// For invalid JSON, the Validate() method should catch it
					err := dv.Validate()
					require.Error(t, err)
				} else {
					// Should create successfully
					client := coderdtest.New(t, &coderdtest.Options{
						DeploymentValues: dv,
					})
					_ = coderdtest.CreateFirstUser(t, client)
				}
			})
		}
	})

	t.Run("TokenCreationWithDifferentRoles", func(t *testing.T) {
		t.Parallel()

		// Test actual token creation with various user role combinations
		// Note: The first user created is an "Owner" (capital O)
		// Global max is 720h (30 days), role-specific limits are lower
		client := createClientWithRoleTokenLifetimes(t, `{
			"owner": "168h",
			"user-admin": "72h",
			"member": "24h"
		}`, 720*time.Hour)

		ctx := context.Background()

		// Get token config to see what the max lifetime is
		tokenConfig, err := client.GetTokenConfig(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("Token config max lifetime: %v (expected 720h - global max)", tokenConfig.MaxTokenLifetime)

		// Test owner can create token up to global max (720h)
		// even though their role limit is 168h, because we use the most generous
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 719 * time.Hour,
		})
		require.NoError(t, err)

		// Test owner cannot exceed global max
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 721 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})

	t.Run("GlobalMaxAsFallback", func(t *testing.T) {
		t.Parallel()

		// Test that users without specific role configs fall back to global max
		client := createClientWithRoleTokenLifetimes(t, `{
			"user-admin": "168h"
		}`, 48*time.Hour)

		ctx := context.Background()

		// The first user is an owner, which isn't in our config
		// So they should fall back to global max (48h)
		// However, since the config isn't being applied during test setup,
		// we need to verify that the role-based config is actually working

		// First, let's check what the actual max is
		tokenConfig, err := client.GetTokenConfig(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("Token config max lifetime: %v", tokenConfig.MaxTokenLifetime)

		// Since "owner" role isn't in the config, should fall back to global max (48h)
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 47 * time.Hour,
		})
		require.NoError(t, err)

		// Should not be able to exceed global max
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 49 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})

	t.Run("RoleSpecificShorterThanGlobal", func(t *testing.T) {
		t.Parallel()

		// Test that when a role-specific limit is shorter than global,
		// the user still gets the most generous lifetime (global max)
		// Note: The first user created is an "Owner" (capital O)
		client := createClientWithRoleTokenLifetimes(t, `{
			"owner": "24h"
		}`, 168*time.Hour) // 7 days global

		ctx := context.Background()

		// Check what the actual max is for this user
		tokenConfig, err := client.GetTokenConfig(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("Token config max lifetime: %v (expected 168h - the global max)", tokenConfig.MaxTokenLifetime)

		// Get the user info to see their roles
		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("User roles: %v", user.Roles)

		// Owner gets the most generous lifetime available (168h global max)
		// even though their role is configured for only 24h
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 167 * time.Hour,
		})
		require.NoError(t, err)

		// But cannot exceed the global max
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 169 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})

	t.Run("OrganizationSpecificRoles", func(t *testing.T) {
		t.Parallel()

		// This test verifies that organization-specific role configurations are parsed correctly
		// Configuration format: "OrgName/role-name": "duration"
		// Note: "default" is the default organization name

		// Set up a client with organization-specific role configurations
		dv := coderdtest.DeploymentValues(t)
		dv.Sessions.MaximumTokenDuration = serpent.Duration(720 * time.Hour)
		dv.Sessions.RoleTokenLifetimes = serpent.String(`{
			"owner": "720h",
			"member": "24h",
			"default/organization-member": "48h",
			"default/organization-admin": "168h"
		}`)

		// Create the client, database, and get the API instance
		client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})

		// Process the role token lifetimes configuration with system context
		ctx := context.Background()
		logger := slog.Make(sloghuman.Sink(io.Discard))
		//nolint:gocritic // Unit test requires system context for database operations
		err := coderd.ProcessRoleTokenLifetimesConfig(dbauthz.AsSystemRestricted(ctx), &api.DeploymentValues.Sessions, api.Database, logger)
		require.NoError(t, err)

		// Create the first user
		firstUser := coderdtest.CreateFirstUser(t, client)

		// Get the user's organization
		//nolint:gocritic // Unit test requires system context for database operations
		orgs, err := api.Database.GetOrganizationsByUserID(dbauthz.AsSystemRestricted(ctx), database.GetOrganizationsByUserIDParams{
			UserID: firstUser.UserID,
		})
		require.NoError(t, err)
		require.Len(t, orgs, 1, "expected user to be in exactly one organization")
		defaultOrg := orgs[0]

		// The config should have been processed at this point, but the org name in the config
		// won't match the actual org, so let's test with a new config that uses the actual org name
		dv.Sessions.RoleTokenLifetimes = serpent.String(fmt.Sprintf(`{
			"owner": "720h",
			"member": "24h",
			"%s/organization-member": "48h",
			"%s/organization-admin": "168h"
		}`, defaultOrg.Name, defaultOrg.Name))

		// Re-process the configuration with the correct org name
		//nolint:gocritic // Unit test requires system context for database operations
		err = coderd.ProcessRoleTokenLifetimesConfig(dbauthz.AsSystemRestricted(ctx), &api.DeploymentValues.Sessions, api.Database, logger)
		require.NoError(t, err)

		// Verify that the configuration was parsed correctly by testing the behavior
		// Site-wide roles should return their configured lifetimes
		ownerRole := rbac.RoleIdentifier{Name: "owner"}
		memberRole := rbac.RoleIdentifier{Name: "member"}
		require.Equal(t, 720*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(ownerRole))
		require.Equal(t, 24*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(memberRole))

		// Organization-specific roles (using internal format "rolename:org_id")
		expectedOrgMemberKey := "organization-member:" + defaultOrg.ID.String()
		expectedOrgAdminKey := "organization-admin:" + defaultOrg.ID.String()

		orgMemberRole, err := rbac.RoleNameFromString(expectedOrgMemberKey)
		require.NoError(t, err)
		orgAdminRole, err := rbac.RoleNameFromString(expectedOrgAdminKey)
		require.NoError(t, err)

		require.Equal(t, 48*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(orgMemberRole))
		require.Equal(t, 168*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(orgAdminRole))

		// Non-existent role should return the global max as fallback
		nonExistentRole := rbac.RoleIdentifier{Name: "non-existent-role"}
		nonExistentRoleLifetime := api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(nonExistentRole)
		require.Equal(t, 720*time.Hour, nonExistentRoleLifetime) // Should fall back to global max
	})

	t.Run("OrganizationRoleWithoutConfig", func(t *testing.T) {
		t.Parallel()

		// Test that users with organization roles not in the config fall back correctly
		// This test verifies the fallback behavior when org-specific roles are not configured

		// Set up a client with only site-wide role configurations (no org-specific roles)
		dv := coderdtest.DeploymentValues(t)
		dv.Sessions.MaximumTokenDuration = serpent.Duration(168 * time.Hour) // 7 days global max
		dv.Sessions.RoleTokenLifetimes = serpent.String(`{
			"owner": "720h",
			"member": "24h"
		}`)

		// Create the client, database, and get the API instance
		client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})

		// Process the role token lifetimes configuration with system context
		ctx := context.Background()
		logger := slog.Make(sloghuman.Sink(io.Discard))
		//nolint:gocritic // Unit test requires system context for database operations
		err := coderd.ProcessRoleTokenLifetimesConfig(dbauthz.AsSystemRestricted(ctx), &api.DeploymentValues.Sessions, api.Database, logger)
		require.NoError(t, err)

		// Create the first user
		_ = coderdtest.CreateFirstUser(t, client)

		// Verify that org-specific roles fall back to global max when not configured
		// For example, "organization-member:some-uuid" should fall back to global max (168h)
		// since it's not in the config
		randomOrgID := "00000000-0000-0000-0000-000000000000"
		orgMemberKey := "organization-member:" + randomOrgID
		orgAdminKey := "organization-admin:" + randomOrgID

		// These org-specific roles are not in the config, so they should fall back to global max
		orgMemberRole, err := rbac.RoleNameFromString(orgMemberKey)
		require.NoError(t, err)
		orgAdminRole, err := rbac.RoleNameFromString(orgAdminKey)
		require.NoError(t, err)
		require.Equal(t, 168*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(orgMemberRole))
		require.Equal(t, 168*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(orgAdminRole))

		// Site-wide roles should still return their configured values
		ownerRole := rbac.RoleIdentifier{Name: "owner"}
		memberRole := rbac.RoleIdentifier{Name: "member"}
		require.Equal(t, 720*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(ownerRole))
		require.Equal(t, 24*time.Hour, api.DeploymentValues.Sessions.MaxTokenLifetimeForRole(memberRole))
	})
}
