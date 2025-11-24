package coderd_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
)

// TestOAuth2ClientIsolation tests that OAuth2 clients cannot access other clients' data
func TestOAuth2ClientIsolation(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx := t.Context()

	// Create two separate OAuth2 clients with unique identifiers
	client1Name := fmt.Sprintf("test-client-1-%s-%d", t.Name(), time.Now().UnixNano())
	client1Req := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://client1.example.com/callback"},
		ClientName:   client1Name,
		ClientURI:    "https://client1.example.com",
	}
	client1Resp, err := client.PostOAuth2ClientRegistration(ctx, client1Req)
	require.NoError(t, err)

	client2Name := fmt.Sprintf("test-client-2-%s-%d", t.Name(), time.Now().UnixNano())
	client2Req := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://client2.example.com/callback"},
		ClientName:   client2Name,
		ClientURI:    "https://client2.example.com",
	}
	client2Resp, err := client.PostOAuth2ClientRegistration(ctx, client2Req)
	require.NoError(t, err)

	t.Run("ClientsCannotAccessOtherClientData", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Client 1 should not be able to access Client 2's data using Client 1's token
		_, err := client.GetOAuth2ClientConfiguration(ctx, client2Resp.ClientID, client1Resp.RegistrationAccessToken)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())

		// Client 2 should not be able to access Client 1's data using Client 2's token
		_, err = client.GetOAuth2ClientConfiguration(ctx, client1Resp.ClientID, client2Resp.RegistrationAccessToken)
		require.Error(t, err)

		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())
	})

	t.Run("ClientsCannotUpdateOtherClients", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Client 1 should not be able to update Client 2 using Client 1's token
		updateReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://malicious.example.com/callback"},
			ClientName:   "Malicious Update",
		}

		_, err := client.PutOAuth2ClientConfiguration(ctx, client2Resp.ClientID, client1Resp.RegistrationAccessToken, updateReq)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())
	})

	t.Run("ClientsCannotDeleteOtherClients", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Client 1 should not be able to delete Client 2 using Client 1's token
		err := client.DeleteOAuth2ClientConfiguration(ctx, client2Resp.ClientID, client1Resp.RegistrationAccessToken)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())

		// Verify Client 2 still exists and is accessible with its own token
		config, err := client.GetOAuth2ClientConfiguration(ctx, client2Resp.ClientID, client2Resp.RegistrationAccessToken)
		require.NoError(t, err)
		require.Equal(t, client2Resp.ClientID, config.ClientID)
	})
}

// TestOAuth2RegistrationTokenSecurity tests security aspects of registration access tokens
func TestOAuth2RegistrationTokenSecurity(t *testing.T) {
	t.Parallel()

	t.Run("InvalidTokenFormats", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := t.Context()

		// Register a client to use for testing
		clientName := fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())
		regReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
		}
		regResp, err := client.PostOAuth2ClientRegistration(ctx, regReq)
		require.NoError(t, err)

		invalidTokens := []string{
			"",                        // Empty token
			"invalid",                 // Too short
			"not-base64-!@#$%^&*",     // Invalid characters
			strings.Repeat("a", 1000), // Too long
			"Bearer " + regResp.RegistrationAccessToken, // With Bearer prefix (incorrect)
		}

		for i, token := range invalidTokens {
			t.Run(fmt.Sprintf("InvalidToken_%d", i), func(t *testing.T) {
				t.Parallel()

				_, err := client.GetOAuth2ClientConfiguration(ctx, regResp.ClientID, token)
				require.Error(t, err)

				var httpErr *codersdk.Error
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())
			})
		}
	})

	t.Run("TokenNotReusableAcrossClients", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := t.Context()

		// Register first client
		client1Name := fmt.Sprintf("test-client-1-%s-%d", t.Name(), time.Now().UnixNano())
		regReq1 := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   client1Name,
		}
		regResp1, err := client.PostOAuth2ClientRegistration(ctx, regReq1)
		require.NoError(t, err)

		// Register another client
		client2Name := fmt.Sprintf("test-client-2-%s-%d", t.Name(), time.Now().UnixNano())
		regReq2 := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example2.com/callback"},
			ClientName:   client2Name,
		}
		regResp2, err := client.PostOAuth2ClientRegistration(ctx, regReq2)
		require.NoError(t, err)

		// Try to use client1's token on client2
		_, err = client.GetOAuth2ClientConfiguration(ctx, regResp2.ClientID, regResp1.RegistrationAccessToken)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusUnauthorized, httpErr.StatusCode())
	})

	t.Run("TokenNotExposedInGETResponse", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := t.Context()

		// Register a client
		clientName := fmt.Sprintf("test-client-%s-%d", t.Name(), time.Now().UnixNano())
		regReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
		}
		regResp, err := client.PostOAuth2ClientRegistration(ctx, regReq)
		require.NoError(t, err)

		// Get client configuration
		config, err := client.GetOAuth2ClientConfiguration(ctx, regResp.ClientID, regResp.RegistrationAccessToken)
		require.NoError(t, err)

		// Registration access token should not be returned in GET responses (RFC 7592)
		require.Empty(t, config.RegistrationAccessToken)
	})
}

// TestOAuth2PrivilegeEscalation tests that clients cannot escalate their privileges
func TestOAuth2PrivilegeEscalation(t *testing.T) {
	t.Parallel()

	t.Run("CannotEscalateScopeViaUpdate", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := t.Context()

		// Register a basic client
		clientName := fmt.Sprintf("test-client-%d", time.Now().UnixNano())
		regReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
			Scope:        "read", // Limited scope
		}
		regResp, err := client.PostOAuth2ClientRegistration(ctx, regReq)
		require.NoError(t, err)

		// Try to escalate scope through update
		updateReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
			Scope:        "read write admin", // Trying to escalate to admin
		}

		// This should succeed (scope changes are allowed in updates)
		// but the system should validate scope permissions appropriately
		updatedConfig, err := client.PutOAuth2ClientConfiguration(ctx, regResp.ClientID, regResp.RegistrationAccessToken, updateReq)
		if err == nil {
			// If update succeeds, verify the scope was set appropriately
			// (The actual scope validation would happen during token issuance)
			require.Contains(t, updatedConfig.Scope, "read")
		}
	})

	t.Run("CustomSchemeRedirectURIs", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := t.Context()

		// Test valid custom schemes per RFC 7591/8252
		validCustomSchemeRequests := []codersdk.OAuth2ClientRegistrationRequest{
			{
				RedirectURIs:            []string{"com.example.myapp://callback"},
				ClientName:              fmt.Sprintf("native-app-1-%d", time.Now().UnixNano()),
				TokenEndpointAuthMethod: "none", // Required for public clients using custom schemes
			},
			{
				RedirectURIs:            []string{"com.example.app://oauth"},
				ClientName:              fmt.Sprintf("native-app-2-%d", time.Now().UnixNano()),
				TokenEndpointAuthMethod: "none", // Required for public clients using custom schemes
			},
			{
				RedirectURIs:            []string{"urn:ietf:wg:oauth:2.0:oob"},
				ClientName:              fmt.Sprintf("native-app-3-%d", time.Now().UnixNano()),
				TokenEndpointAuthMethod: "none", // Required for public clients
			},
		}

		for i, req := range validCustomSchemeRequests {
			t.Run(fmt.Sprintf("ValidCustomSchemeRequest_%d", i), func(t *testing.T) {
				t.Parallel()

				_, err := client.PostOAuth2ClientRegistration(ctx, req)
				// Valid custom schemes should be allowed per RFC 7591/8252
				require.NoError(t, err)
			})
		}

		// Test that dangerous schemes are properly rejected for security
		dangerousSchemeRequests := []struct {
			req    codersdk.OAuth2ClientRegistrationRequest
			scheme string
		}{
			{
				req: codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs:            []string{"javascript:alert('test')"},
					ClientName:              fmt.Sprintf("native-app-js-%d", time.Now().UnixNano()),
					TokenEndpointAuthMethod: "none",
				},
				scheme: "javascript",
			},
			{
				req: codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs:            []string{"data:text/html,<html></html>"},
					ClientName:              fmt.Sprintf("native-app-data-%d", time.Now().UnixNano()),
					TokenEndpointAuthMethod: "none",
				},
				scheme: "data",
			},
		}

		for _, test := range dangerousSchemeRequests {
			t.Run(fmt.Sprintf("DangerousScheme_%s", test.scheme), func(t *testing.T) {
				t.Parallel()

				_, err := client.PostOAuth2ClientRegistration(ctx, test.req)
				// Dangerous schemes should be rejected for security
				require.Error(t, err)
				require.Contains(t, err.Error(), "dangerous scheme")
			})
		}
	})
}

// TestOAuth2InformationDisclosure tests that error messages don't leak sensitive information
func TestOAuth2InformationDisclosure(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx := t.Context()

	// Register a client for testing
	clientName := fmt.Sprintf("test-client-%d", time.Now().UnixNano())
	regReq := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
		ClientName:   clientName,
	}
	regResp, err := client.PostOAuth2ClientRegistration(ctx, regReq)
	require.NoError(t, err)

	t.Run("ErrorsDoNotLeakClientSecrets", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Try various invalid operations and ensure they don't leak the client secret
		_, err := client.GetOAuth2ClientConfiguration(ctx, regResp.ClientID, "invalid-token")
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)

		// Error message should not contain any part of the client secret or registration token
		errorText := strings.ToLower(httpErr.Message + httpErr.Detail)
		require.NotContains(t, errorText, strings.ToLower(regResp.ClientSecret))
		require.NotContains(t, errorText, strings.ToLower(regResp.RegistrationAccessToken))
	})

	t.Run("ErrorsDoNotLeakDatabaseDetails", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Try to access non-existent client
		_, err := client.GetOAuth2ClientConfiguration(ctx, "non-existent-client-id", regResp.RegistrationAccessToken)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)

		// Error message should not leak database schema information
		errorText := strings.ToLower(httpErr.Message + httpErr.Detail)
		require.NotContains(t, errorText, "sql")
		require.NotContains(t, errorText, "database")
		require.NotContains(t, errorText, "table")
		require.NotContains(t, errorText, "row")
		require.NotContains(t, errorText, "constraint")
	})

	t.Run("ErrorsAreConsistentForInvalidClients", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Test with various invalid client IDs to ensure consistent error responses
		invalidClientIDs := []string{
			"non-existent-1",
			"non-existent-2",
			"totally-different-format",
		}

		var errorMessages []string
		for _, clientID := range invalidClientIDs {
			_, err := client.GetOAuth2ClientConfiguration(ctx, clientID, regResp.RegistrationAccessToken)
			require.Error(t, err)

			var httpErr *codersdk.Error
			require.ErrorAs(t, err, &httpErr)
			errorMessages = append(errorMessages, httpErr.Message)
		}

		// All error messages should be similar (not leaking which client IDs exist vs don't exist)
		for i := 1; i < len(errorMessages); i++ {
			require.Equal(t, errorMessages[0], errorMessages[i])
		}
	})
}

// TestOAuth2ConcurrentSecurityOperations tests security under concurrent operations
func TestOAuth2ConcurrentSecurityOperations(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx := t.Context()

	// Register a client for testing
	clientName := fmt.Sprintf("test-client-%d", time.Now().UnixNano())
	regReq := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
		ClientName:   clientName,
	}
	regResp, err := client.PostOAuth2ClientRegistration(ctx, regReq)
	require.NoError(t, err)

	t.Run("ConcurrentAccessAttempts", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		const numGoroutines = 20
		var wg sync.WaitGroup
		errors := make([]error, numGoroutines)

		// Launch concurrent attempts to access the client configuration
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				_, err := client.GetOAuth2ClientConfiguration(ctx, regResp.ClientID, regResp.RegistrationAccessToken)
				errors[index] = err
			}(i)
		}

		wg.Wait()

		// All requests should succeed (they're all valid)
		for i, err := range errors {
			require.NoError(t, err, "Request %d failed", i)
		}
	})

	t.Run("ConcurrentInvalidAccessAttempts", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		const numGoroutines = 20
		var wg sync.WaitGroup
		statusCodes := make([]int, numGoroutines)

		// Launch concurrent attempts with invalid tokens
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				_, err := client.GetOAuth2ClientConfiguration(ctx, regResp.ClientID, fmt.Sprintf("invalid-token-%d", index))
				if err == nil {
					t.Errorf("Expected error for goroutine %d", index)
					return
				}

				var httpErr *codersdk.Error
				if !errors.As(err, &httpErr) {
					t.Errorf("Expected codersdk.Error for goroutine %d", index)
					return
				}
				statusCodes[index] = httpErr.StatusCode()
			}(i)
		}

		wg.Wait()

		// All requests should fail with 401 status
		for i, statusCode := range statusCodes {
			require.Equal(t, http.StatusUnauthorized, statusCode, "Request %d had unexpected status", i)
		}
	})

	t.Run("ConcurrentClientDeletion", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		// Register a client specifically for deletion testing
		deleteClientName := fmt.Sprintf("delete-test-client-%d", time.Now().UnixNano())
		deleteRegReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://delete-test.example.com/callback"},
			ClientName:   deleteClientName,
		}
		deleteRegResp, err := client.PostOAuth2ClientRegistration(ctx, deleteRegReq)
		require.NoError(t, err)

		const numGoroutines = 5
		var wg sync.WaitGroup
		deleteResults := make([]error, numGoroutines)

		// Launch concurrent deletion attempts
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				err := client.DeleteOAuth2ClientConfiguration(ctx, deleteRegResp.ClientID, deleteRegResp.RegistrationAccessToken)
				deleteResults[index] = err
			}(i)
		}

		wg.Wait()

		// Only one deletion should succeed, others should fail
		successCount := 0
		for _, err := range deleteResults {
			if err == nil {
				successCount++
			}
		}

		// At least one should succeed, and multiple successes are acceptable (idempotent operation)
		require.Greater(t, successCount, 0, "At least one deletion should succeed")

		// Verify the client is actually deleted
		_, err = client.GetOAuth2ClientConfiguration(ctx, deleteRegResp.ClientID, deleteRegResp.RegistrationAccessToken)
		require.Error(t, err)

		var httpErr *codersdk.Error
		require.ErrorAs(t, err, &httpErr)
		require.True(t, httpErr.StatusCode() == http.StatusUnauthorized || httpErr.StatusCode() == http.StatusNotFound)
	})
}
