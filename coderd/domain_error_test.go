package coderd_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
)

// TestOIDCDomainErrorMessage ensures that when a user with an unauthorized domain
// attempts to login, the error message doesn't expose the list of authorized domains.
func TestOIDCDomainErrorMessage(t *testing.T) {
	t.Parallel()

	// Setup OIDC fake provider
	fake := oidctest.NewFakeIDP(t, oidctest.WithServing())

	// Configure OIDC provider with domain restrictions
	allowedDomains := []string{"allowed1.com", "allowed2.org", "company.internal"}
	cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
		cfg.EmailDomain = allowedDomains
		cfg.AllowSignups = true
	})

	// Create a Coder server with OIDC enabled
	server := coderdtest.New(t, &coderdtest.Options{
		OIDCConfig: cfg,
	})

	// Test case 1: Email domain not in allowed list
	t.Run("ErrorMessageOmitsDomains", func(t *testing.T) {
		t.Parallel()

		// Prepare claims with email from unauthorized domain
		claims := jwt.MapClaims{
			"email":          "user@unauthorized.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}

		// Attempt login and check for failure
		_, resp := fake.AttemptLogin(t, server, claims)
		defer resp.Body.Close()

		// Verify the status code
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		// Check the response content
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify the message contains the generic text
		require.Contains(t, string(data), "is not from an authorized domain")
		require.Contains(t, string(data), "Please contact your administrator")

		// Verify it doesn't contain any of the allowed domains
		for _, domain := range allowedDomains {
			require.NotContains(t, string(data), domain)
		}
	})

	// Test case 2: Malformed email without @ symbol
	t.Run("MalformedEmailErrorOmitsDomains", func(t *testing.T) {
		t.Parallel()

		// Prepare claims with an invalid email format (no @ symbol)
		claims := jwt.MapClaims{
			"email":          "invalid-email-without-domain",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}

		// Attempt login and check for failure
		_, resp := fake.AttemptLogin(t, server, claims)
		defer resp.Body.Close()

		// Verify the status code
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		// Check the response content
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify the message contains the generic text
		require.Contains(t, string(data), "is not from an authorized domain")
		require.Contains(t, string(data), "Please contact your administrator")

		// Verify it doesn't contain any of the allowed domains
		for _, domain := range allowedDomains {
			require.NotContains(t, string(data), domain)
		}
	})

	// Test case 3: Authorized domain (should succeed)
	t.Run("AuthorizedDomainSucceeds", func(t *testing.T) {
		t.Parallel()

		// Prepare claims with an authorized domain
		claims := jwt.MapClaims{
			"email":          "user@allowed1.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}

		// Attempt login and expect success
		client, resp := fake.Login(t, server, claims)
		defer resp.Body.Close()

		// Verify the user was created correctly
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "user", user.Username)
	})
}
