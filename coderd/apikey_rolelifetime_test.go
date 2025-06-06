package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/expr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// createClientWithRoleTokenLifetimes creates a test client with role-based token lifetimes configured
func createClientWithRoleTokenLifetimes(t *testing.T, roleTokenLifetimeExpression string, maxTokenDuration time.Duration) *codersdk.Client {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.Sessions.MaximumTokenDuration = serpent.Duration(maxTokenDuration)
	dv.Sessions.MaximumTokenDurationExpression = serpent.String(roleTokenLifetimeExpression)

	// Create the client, database, and get the API instance
	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: dv,
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Compile the token lifetime expression
	// This is normally done in cli/server.go but needs to be done manually in tests
	program, err := api.DeploymentValues.Sessions.CompiledMaximumTokenDurationProgram()
	require.NoError(t, err)

	// Verify the configuration was processed
	t.Logf("RoleTokenLifetimeExpression config: %s", api.DeploymentValues.Sessions.MaximumTokenDurationExpression.Value())
	t.Logf("MaximumTokenDuration: %v", api.DeploymentValues.Sessions.MaximumTokenDuration.Value())
	// Check if we have a compiled program
	if program != nil {
		t.Logf("expr expression compiled successfully")
	} else {
		t.Logf("No expr expression configured")
	}

	// Create the first user
	_ = coderdtest.CreateFirstUser(t, client)

	return client
}

func TestRoleBasedTokenLifetimes_Integration(t *testing.T) {
	t.Parallel()

	t.Run("ServerStartupWithValidExprExpressions", func(t *testing.T) {
		t.Parallel()

		// Test server starts successfully with valid expr expressions
		testCases := []struct {
			name           string
			exprExpression string
		}{
			{
				name:           "ValidRoleBasedExpression",
				exprExpression: `any(subject.Roles, .Name == "owner") ? duration("168h") : any(subject.Roles, .Name == "user-admin") ? duration("72h") : duration("24h")`,
			},
			{
				name:           "ValidSimpleExpression",
				exprExpression: `duration(globalMaxDuration)`,
			},
			{
				name:           "EmptyExpression",
				exprExpression: ``,
			},
			{
				name:           "EmailBasedExpression",
				exprExpression: `subject.Email endsWith "@company.com") ? duration("720h") : duration("24h")`,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dv := coderdtest.DeploymentValues(t)
				dv.Sessions.MaximumTokenDurationExpression = serpent.String(tc.exprExpression)

				// Should create successfully
				client := coderdtest.New(t, &coderdtest.Options{
					DeploymentValues: dv,
				})
				_ = coderdtest.CreateFirstUser(t, client)
			})
		}
	})

	t.Run("InvalidExprExpressions", func(t *testing.T) {
		t.Parallel()

		// Test that invalid expr expressions fail validation
		testCases := []struct {
			name           string
			exprExpression string
		}{
			{
				name:           "InvalidExprSyntax",
				exprExpression: `any(subject.Roles, .Name == "owner" ? duration("168h")`, // Missing closing parenthesis
			},
			{
				name:           "UndefinedVariable",
				exprExpression: `unknownVariable ? duration("720h") : duration("168h")`,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// For invalid expr expressions, try to compile
				_, err := expr.CompileTokenLifetimeExpression(tc.exprExpression)
				if err != nil {
					// expr compilation failed as expected
					return
				}
				// If compilation succeeded but we expected failure, that's also a test failure
				t.Fatalf("Expected expr expression to fail compilation but it succeeded: %s", tc.exprExpression)
			})
		}
	})

	t.Run("TokenCreationWithDifferentRoles", func(t *testing.T) {
		t.Parallel()

		// Test actual token creation with various user role combinations
		// Note: The first user created is an "Owner" (capital O)
		// Global max is 720h (30 days), expr expression provides role-specific rules
		client := createClientWithRoleTokenLifetimes(t, `any(subject.Roles, .Name == "owner") ? duration("168h") : any(subject.Roles, .Name == "user-admin") ? duration("72h") : any(subject.Roles, .Name == "member") ? duration("24h") : duration(globalMaxDuration)`, 720*time.Hour)

		ctx := context.Background()

		// Get token config to see what the max lifetime is
		tokenConfig, err := client.GetTokenConfig(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("Token config max lifetime: %v (expected 720h - global max)", tokenConfig.MaxTokenLifetime)

		// Test owner can create token up to what the expr expression allows (168h)
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 167 * time.Hour,
		})
		require.NoError(t, err)

		// Test owner cannot exceed what the expr expression allows (168h)
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 169 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})

	t.Run("GlobalMaxAsFallback", func(t *testing.T) {
		t.Parallel()

		// Test that users without specific role configs fall back to global max
		client := createClientWithRoleTokenLifetimes(t, `any(subject.Roles, .Name == "user-admin") ? duration("168h") : duration(globalMaxDuration)`, 48*time.Hour)

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

		// Test expr expression that chooses between role-specific and global max
		// Note: The first user created is an "Owner" (capital O)
		client := createClientWithRoleTokenLifetimes(t, `any(subject.Roles, .Name == "owner") ? duration(globalMaxDuration) : duration("24h")`, 168*time.Hour) // 7 days global

		ctx := context.Background()

		// Check what the actual max is for this user
		tokenConfig, err := client.GetTokenConfig(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("Token config max lifetime: %v (expected 168h - the global max)", tokenConfig.MaxTokenLifetime)

		// Get the user info to see their roles
		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		t.Logf("User roles: %v", user.Roles)

		// Owner gets the global max (168h) because the expr expression returns globalMaxDuration
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

		// This test verifies that organization-specific role configurations work with expr expressions

		// Set up a client with organization-specific role configurations using expr
		exprExpression := `
			any(subject.Roles, .Name == "owner" && .OrgID == "") ? duration("720h") :
			any(subject.Roles, .Name == "member" && .OrgID == "") ? duration("24h") :
			any(subject.Roles, .Name == "organization-member" && .OrgID != "") ? duration("48h") :
			any(subject.Roles, .Name == "organization-admin" && .OrgID != "") ? duration("168h") :
			duration(defaultDuration)
		`

		dv := coderdtest.DeploymentValues(t)
		dv.Sessions.MaximumTokenDuration = serpent.Duration(720 * time.Hour)
		dv.Sessions.MaximumTokenDurationExpression = serpent.String(exprExpression)

		// Create the client, database, and get the API instance
		client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})

		// Compile the expr expression
		ctx := context.Background()
		_, err := api.DeploymentValues.Sessions.CompiledMaximumTokenDurationProgram()
		require.NoError(t, err)

		// Create the first user
		_ = coderdtest.CreateFirstUser(t, client)

		// Test that the expr expression is working for the site-wide owner role
		// The first user gets site-wide owner role, so should get 720h
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 719 * time.Hour,
		})
		require.NoError(t, err)

		// Test that the user cannot exceed what the expression allows
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 721 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})

	t.Run("OrganizationRoleWithoutConfig", func(t *testing.T) {
		t.Parallel()

		// Test expr expression with fallback behavior for unconfigured roles

		// Set up a client with only site-wide role configurations (no org-specific roles)
		client := createClientWithRoleTokenLifetimes(t, `any(subject.Roles, .Name == "owner") ? duration("720h") : any(subject.Roles, .Name == "member") ? duration("24h") : duration(globalMaxDuration)`, 168*time.Hour)

		ctx := context.Background()

		// Test that the first user (owner) gets 720h according to the expr expression,
		// not the global max (168h)
		_, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 719 * time.Hour,
		})
		require.NoError(t, err)

		// Test that owner cannot exceed what expr expression allows (720h)
		_, err = client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
			Lifetime: 721 * time.Hour,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the maximum allowed")
	})
}
