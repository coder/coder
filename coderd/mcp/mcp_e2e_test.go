package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPHTTP_E2E_ClientIntegration(t *testing.T) {
	t.Parallel()

	// Setup Coder server with authentication
	coderClient, closer, api := coderdtest.NewWithAPI(t, nil)
	defer closer.Close()

	_ = coderdtest.CreateFirstUser(t, coderClient)

	// Create MCP client pointing to our endpoint
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint

	// Configure client with authentication headers using RFC 6750 Bearer token
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + coderClient.SessionToken(),
		}))
	require.NoError(t, err)
	defer func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Start client
	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	// Initialize connection
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
		},
	}

	result, err := mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)
	require.Equal(t, mcpserver.MCPServerName, result.ServerInfo.Name)
	require.Equal(t, mcp.LATEST_PROTOCOL_VERSION, result.ProtocolVersion)
	require.NotNil(t, result.Capabilities)

	// Test tool listing
	tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, tools.Tools)

	// Verify we have some expected Coder tools
	var foundTools []string
	for _, tool := range tools.Tools {
		foundTools = append(foundTools, tool.Name)
	}

	// Check for some basic tools that should be available
	assert.Contains(t, foundTools, toolsdk.ToolNameGetAuthenticatedUser, "Should have authenticated user tool")

	// Find and execute the authenticated user tool
	var userTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == toolsdk.ToolNameGetAuthenticatedUser {
			userTool = &tool
			break
		}
	}
	require.NotNil(t, userTool, "Expected to find "+toolsdk.ToolNameGetAuthenticatedUser+" tool")

	// Execute the tool
	toolReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      userTool.Name,
			Arguments: map[string]any{},
		},
	}

	toolResult, err := mcpClient.CallTool(ctx, toolReq)
	require.NoError(t, err)
	require.NotEmpty(t, toolResult.Content)

	// Verify the result contains user information
	assert.Len(t, toolResult.Content, 1)
	if textContent, ok := toolResult.Content[0].(mcp.TextContent); ok {
		assert.Equal(t, "text", textContent.Type)
		assert.NotEmpty(t, textContent.Text)
	} else {
		t.Errorf("Expected TextContent type, got %T", toolResult.Content[0])
	}

	// Test ping functionality
	err = mcpClient.Ping(ctx)
	require.NoError(t, err)
}

func TestMCPHTTP_E2E_UnauthenticatedAccess(t *testing.T) {
	t.Parallel()

	// Setup Coder server
	_, closer, api := coderdtest.NewWithAPI(t, nil)
	defer closer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Test direct HTTP request to verify 401 status code
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint

	// Make a POST request without authentication (MCP over HTTP uses POST)
	//nolint:gosec // Test code using controlled localhost URL
	req, err := http.NewRequestWithContext(ctx, "POST", mcpURL, strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}`))
	require.NoError(t, err, "Should be able to create HTTP request")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "Should be able to make HTTP request")
	defer resp.Body.Close()

	// Verify we get 401 Unauthorized
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should get HTTP 401 for unauthenticated access")

	// Also test with MCP client to ensure it handles the error gracefully
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL)
	require.NoError(t, err, "Should be able to create MCP client without authentication")
	defer func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	}()

	// Start client and try to initialize - this should fail due to authentication
	err = mcpClient.Start(ctx)
	if err != nil {
		// Authentication failed at transport level - this is expected
		t.Logf("Unauthenticated access test successful: Transport-level authentication error: %v", err)
		return
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client-unauth",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	require.Error(t, err, "Should fail during MCP initialization without authentication")
}

func TestMCPHTTP_E2E_ToolWithWorkspace(t *testing.T) {
	// Setup Coder server with full workspace environment
	coderClient, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	defer closer.Close()

	user := coderdtest.CreateFirstUser(t, coderClient)

	// Create template and workspace for testing
	version := coderdtest.CreateTemplateVersion(t, coderClient, user.OrganizationID, nil)
	awaitTemplateVersionJobCompleted(t, coderClient, version.ID)
	template := coderdtest.CreateTemplate(t, coderClient, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, coderClient, template.ID)

	// Create MCP client
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + coderClient.SessionToken(),
		}))
	require.NoError(t, err)
	defer func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Start and initialize client
	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client-workspace",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	// Test workspace-related tools
	tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)

	// Find workspace listing tool
	var workspaceTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == toolsdk.ToolNameListWorkspaces {
			workspaceTool = &tool
			break
		}
	}

	if workspaceTool != nil {
		// Execute workspace listing tool
		toolReq := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      workspaceTool.Name,
				Arguments: map[string]any{},
			},
		}

		toolResult, err := mcpClient.CallTool(ctx, toolReq)
		require.NoError(t, err)
		require.NotEmpty(t, toolResult.Content)

		// Verify the result mentions our workspace
		if textContent, ok := toolResult.Content[0].(mcp.TextContent); ok {
			assert.Contains(t, textContent.Text, workspace.Name, "Workspace listing should include our test workspace")
		} else {
			t.Error("Expected TextContent type from workspace tool")
		}

		t.Logf("Workspace tool test successful: Found workspace %s in results", workspace.Name)
	} else {
		t.Skip("Workspace listing tool not available, skipping workspace-specific test")
	}
}

func TestMCPHTTP_E2E_ErrorHandling(t *testing.T) {
	t.Parallel()

	// Setup Coder server
	coderClient, closer, api := coderdtest.NewWithAPI(t, nil)
	defer closer.Close()

	_ = coderdtest.CreateFirstUser(t, coderClient)

	// Create MCP client
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + coderClient.SessionToken(),
		}))
	require.NoError(t, err)
	defer func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Start and initialize client
	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client-errors",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	// Test calling non-existent tool
	toolReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "nonexistent_tool",
			Arguments: map[string]any{},
		},
	}

	_, err = mcpClient.CallTool(ctx, toolReq)
	require.Error(t, err, "Should get error when calling non-existent tool")
	require.Contains(t, err.Error(), "nonexistent_tool", "Should mention the tool name in error message")

	t.Logf("Error handling test successful: Got expected error for non-existent tool")
}

func TestMCPHTTP_E2E_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	// Setup Coder server
	coderClient, closer, api := coderdtest.NewWithAPI(t, nil)
	defer closer.Close()

	_ = coderdtest.CreateFirstUser(t, coderClient)

	// Create MCP client
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + coderClient.SessionToken(),
		}))
	require.NoError(t, err)
	defer func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	// Start and initialize client
	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client-concurrent",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)

	// Test concurrent tool listings
	const numConcurrent = 5
	eg, egCtx := errgroup.WithContext(ctx)

	for range numConcurrent {
		eg.Go(func() error {
			reqCtx, reqCancel := context.WithTimeout(egCtx, testutil.WaitLong)
			defer reqCancel()

			tools, err := mcpClient.ListTools(reqCtx, mcp.ListToolsRequest{})
			if err != nil {
				return err
			}

			if len(tools.Tools) == 0 {
				return assert.AnError
			}

			return nil
		})
	}

	// Wait for all concurrent requests to complete
	err = eg.Wait()
	require.NoError(t, err, "All concurrent requests should succeed")

	t.Logf("Concurrent requests test successful: All %d requests completed successfully", numConcurrent)
}

func TestMCPHTTP_E2E_RFC6750_UnauthenticatedRequest(t *testing.T) {
	t.Parallel()

	// Setup Coder server
	_, closer, api := coderdtest.NewWithAPI(t, nil)
	defer closer.Close()

	// Make a request without any authentication headers
	req := &http.Request{
		Method: "POST",
		URL:    mustParseURL(t, api.AccessURL.String()+mcpserver.MCPEndpoint),
		Header: make(http.Header),
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should get 401 Unauthorized
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// RFC 6750 requires WWW-Authenticate header on 401 responses
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	require.NotEmpty(t, wwwAuth, "RFC 6750 requires WWW-Authenticate header for 401 responses")
	require.Contains(t, wwwAuth, "Bearer", "WWW-Authenticate header should indicate Bearer authentication")
	require.Contains(t, wwwAuth, `realm="coder"`, "WWW-Authenticate header should include realm")

	t.Logf("RFC 6750 WWW-Authenticate header test successful: %s", wwwAuth)
}

func TestMCPHTTP_E2E_OAuth2_EndToEnd(t *testing.T) {
	t.Parallel()

	// Setup Coder server with OAuth2 provider enabled
	coderClient, closer, api := coderdtest.NewWithAPI(t, nil)
	t.Cleanup(func() { closer.Close() })

	_ = coderdtest.CreateFirstUser(t, coderClient)

	ctx := t.Context()

	// Create OAuth2 app (for demonstration that OAuth2 provider is working)
	_, err := coderClient.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
		Name:        "test-mcp-app",
		CallbackURL: "http://localhost:3000/callback",
	})
	require.NoError(t, err)

	// Test 1: OAuth2 Token Endpoint Error Format
	t.Run("OAuth2TokenEndpointErrorFormat", func(t *testing.T) {
		t.Parallel()
		// Test that the /oauth2/tokens endpoint responds with proper OAuth2 error format
		// Note: The endpoint is /oauth2/tokens (plural), not /oauth2/token (singular)
		req := &http.Request{
			Method: "POST",
			URL:    mustParseURL(t, api.AccessURL.String()+"/oauth2/tokens"),
			Header: map[string][]string{
				"Content-Type": {"application/x-www-form-urlencoded"},
			},
			Body: http.NoBody,
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// The OAuth2 token endpoint should return HTTP 400 for invalid requests
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Read and verify the response is OAuth2-compliant JSON error format
		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		t.Logf("OAuth2 tokens endpoint returned status: %d, body: %q", resp.StatusCode, string(bodyBytes))

		// Should be valid JSON with OAuth2 error format
		var errorResponse map[string]any
		err = json.Unmarshal(bodyBytes, &errorResponse)
		require.NoError(t, err, "Response should be valid JSON")

		// Verify OAuth2 error format (RFC 6749 section 5.2)
		require.NotEmpty(t, errorResponse["error"], "Error field should not be empty")
	})

	// Test 2: MCP with OAuth2 Bearer Token
	t.Run("MCPWithOAuth2BearerToken", func(t *testing.T) {
		t.Parallel()
		// For this test, we'll use the user's regular session token formatted as a Bearer token
		// In a real OAuth2 flow, this would be an OAuth2 access token
		sessionToken := coderClient.SessionToken()

		mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
		mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + sessionToken,
			}))
		require.NoError(t, err)
		defer func() {
			if closeErr := mcpClient.Close(); closeErr != nil {
				t.Logf("Failed to close MCP client: %v", closeErr)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Start and initialize MCP client with Bearer token
		err = mcpClient.Start(ctx)
		require.NoError(t, err)

		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-oauth2-client",
					Version: "1.0.0",
				},
			},
		}

		result, err := mcpClient.Initialize(ctx, initReq)
		require.NoError(t, err)
		require.Equal(t, mcpserver.MCPServerName, result.ServerInfo.Name)

		// Test tool listing with OAuth2 Bearer token
		tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, tools.Tools)

		t.Logf("OAuth2 Bearer token MCP test successful: Found %d tools", len(tools.Tools))
	})

	// Test 3: Full OAuth2 Authorization Code Flow with Token Refresh
	t.Run("OAuth2FullFlowWithTokenRefresh", func(t *testing.T) {
		t.Parallel()
		// Create an OAuth2 app specifically for this test
		app, err := coderClient.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "test-oauth2-flow-app",
			CallbackURL: "http://localhost:3000/callback",
		})
		require.NoError(t, err)

		// Create a client secret for the app
		secret, err := coderClient.PostOAuth2ProviderAppSecret(ctx, app.ID)
		require.NoError(t, err)

		// Step 1: Simulate authorization code flow by creating an authorization code
		// In a real flow, this would be done through the browser consent page
		// For testing, we'll create the code directly using the internal API

		// First, we need to authorize the app (simulating user consent)
		authURL := fmt.Sprintf("%s/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=%s&state=test_state",
			api.AccessURL.String(), app.ID, "http://localhost:3000/callback")

		// Create an HTTP client that follows redirects but captures the final redirect
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Stop following redirects
			},
		}

		// Make the authorization request (this would normally be done in a browser)
		req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
		require.NoError(t, err)
		// Use RFC 6750 Bearer token for authentication
		req.Header.Set("Authorization", "Bearer "+coderClient.SessionToken())

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// The response should be a redirect to the consent page or directly to callback
		// For testing purposes, let's simulate the POST consent approval
		if resp.StatusCode == http.StatusOK {
			// This means we got the consent page, now we need to POST consent
			consentReq, err := http.NewRequestWithContext(ctx, "POST", authURL, nil)
			require.NoError(t, err)
			consentReq.Header.Set("Authorization", "Bearer "+coderClient.SessionToken())
			consentReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err = client.Do(consentReq)
			require.NoError(t, err)
			defer resp.Body.Close()
		}

		// Extract authorization code from redirect URL
		require.True(t, resp.StatusCode >= 300 && resp.StatusCode < 400, "Expected redirect response")
		location := resp.Header.Get("Location")
		require.NotEmpty(t, location, "Expected Location header in redirect")

		redirectURL, err := url.Parse(location)
		require.NoError(t, err)
		authCode := redirectURL.Query().Get("code")
		require.NotEmpty(t, authCode, "Expected authorization code in redirect URL")

		t.Logf("Successfully obtained authorization code: %s", authCode[:10]+"...")

		// Step 2: Exchange authorization code for access token and refresh token
		tokenRequestBody := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {app.ID.String()},
			"client_secret": {secret.ClientSecretFull},
			"code":          {authCode},
			"redirect_uri":  {"http://localhost:3000/callback"},
		}

		tokenReq, err := http.NewRequestWithContext(ctx, "POST", api.AccessURL.String()+"/oauth2/tokens",
			strings.NewReader(tokenRequestBody.Encode()))
		require.NoError(t, err)
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		tokenResp, err := client.Do(tokenReq)
		require.NoError(t, err)
		defer tokenResp.Body.Close()

		require.Equal(t, http.StatusOK, tokenResp.StatusCode, "Token exchange should succeed")

		// Parse token response
		var tokenResponse map[string]any
		err = json.NewDecoder(tokenResp.Body).Decode(&tokenResponse)
		require.NoError(t, err)

		accessToken, ok := tokenResponse["access_token"].(string)
		require.True(t, ok, "Response should contain access_token")
		require.NotEmpty(t, accessToken)

		refreshToken, ok := tokenResponse["refresh_token"].(string)
		require.True(t, ok, "Response should contain refresh_token")
		require.NotEmpty(t, refreshToken)

		tokenType, ok := tokenResponse["token_type"].(string)
		require.True(t, ok, "Response should contain token_type")
		require.Equal(t, "Bearer", tokenType)

		t.Logf("Successfully obtained access token: %s...", accessToken[:10])
		t.Logf("Successfully obtained refresh token: %s...", refreshToken[:10])

		// Step 3: Use access token to authenticate with MCP endpoint
		mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
		mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + accessToken,
			}))
		require.NoError(t, err)
		defer func() {
			if closeErr := mcpClient.Close(); closeErr != nil {
				t.Logf("Failed to close MCP client: %v", closeErr)
			}
		}()

		// Initialize and test the MCP connection with OAuth2 access token
		err = mcpClient.Start(ctx)
		require.NoError(t, err)

		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-oauth2-flow-client",
					Version: "1.0.0",
				},
			},
		}

		result, err := mcpClient.Initialize(ctx, initReq)
		require.NoError(t, err)
		require.Equal(t, mcpserver.MCPServerName, result.ServerInfo.Name)

		// Test tool execution with OAuth2 access token
		tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, tools.Tools)

		// Find and execute the authenticated user tool
		var userTool *mcp.Tool
		for _, tool := range tools.Tools {
			if tool.Name == toolsdk.ToolNameGetAuthenticatedUser {
				userTool = &tool
				break
			}
		}
		require.NotNil(t, userTool, "Expected to find "+toolsdk.ToolNameGetAuthenticatedUser+" tool")

		toolReq := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      userTool.Name,
				Arguments: map[string]any{},
			},
		}

		toolResult, err := mcpClient.CallTool(ctx, toolReq)
		require.NoError(t, err)
		require.NotEmpty(t, toolResult.Content)

		t.Logf("Successfully executed tool with OAuth2 access token")

		// Step 4: Refresh the access token using refresh token
		refreshRequestBody := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {app.ID.String()},
			"client_secret": {secret.ClientSecretFull},
			"refresh_token": {refreshToken},
		}

		refreshReq, err := http.NewRequestWithContext(ctx, "POST", api.AccessURL.String()+"/oauth2/tokens",
			strings.NewReader(refreshRequestBody.Encode()))
		require.NoError(t, err)
		refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		refreshResp, err := client.Do(refreshReq)
		require.NoError(t, err)
		defer refreshResp.Body.Close()

		require.Equal(t, http.StatusOK, refreshResp.StatusCode, "Token refresh should succeed")

		// Parse refresh response
		var refreshResponse map[string]any
		err = json.NewDecoder(refreshResp.Body).Decode(&refreshResponse)
		require.NoError(t, err)

		newAccessToken, ok := refreshResponse["access_token"].(string)
		require.True(t, ok, "Refresh response should contain new access_token")
		require.NotEmpty(t, newAccessToken)
		require.NotEqual(t, accessToken, newAccessToken, "New access token should be different")

		newRefreshToken, ok := refreshResponse["refresh_token"].(string)
		require.True(t, ok, "Refresh response should contain new refresh_token")
		require.NotEmpty(t, newRefreshToken)

		t.Logf("Successfully refreshed token: %s...", newAccessToken[:10])

		// Step 5: Use new access token to create another MCP connection
		newMcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + newAccessToken,
			}))
		require.NoError(t, err)
		defer func() {
			if closeErr := newMcpClient.Close(); closeErr != nil {
				t.Logf("Failed to close new MCP client: %v", closeErr)
			}
		}()

		// Test the new token works
		err = newMcpClient.Start(ctx)
		require.NoError(t, err)

		newInitReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-refreshed-token-client",
					Version: "1.0.0",
				},
			},
		}

		newResult, err := newMcpClient.Initialize(ctx, newInitReq)
		require.NoError(t, err)
		require.Equal(t, mcpserver.MCPServerName, newResult.ServerInfo.Name)

		// Verify we can still execute tools with the refreshed token
		newTools, err := newMcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, newTools.Tools)

		t.Logf("OAuth2 full flow test successful: app creation -> authorization -> token exchange -> MCP usage -> token refresh -> MCP usage with refreshed token")
	})

	// Test 4: Invalid Bearer Token
	t.Run("InvalidBearerToken", func(t *testing.T) {
		t.Parallel()
		req := &http.Request{
			Method: "POST",
			URL:    mustParseURL(t, api.AccessURL.String()+mcpserver.MCPEndpoint),
			Header: map[string][]string{
				"Authorization": {"Bearer invalid_token_value"},
				"Content-Type":  {"application/json"},
			},
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should get 401 Unauthorized
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		// Should have RFC 6750 compliant WWW-Authenticate header
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth)
		require.Contains(t, wwwAuth, "Bearer")
		require.Contains(t, wwwAuth, `realm="coder"`)
		require.Contains(t, wwwAuth, "invalid_token")

		t.Logf("Invalid Bearer token test successful: %s", wwwAuth)
	})

	// Test 5: Dynamic Client Registration with Unauthenticated MCP Access
	t.Run("DynamicClientRegistrationWithMCPFlow", func(t *testing.T) {
		t.Parallel()
		// Step 1: Attempt unauthenticated MCP access
		mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint
		req := &http.Request{
			Method: "POST",
			URL:    mustParseURL(t, mcpURL),
			Header: make(http.Header),
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should get 401 Unauthorized with WWW-Authenticate header
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		require.NotEmpty(t, wwwAuth, "RFC 6750 requires WWW-Authenticate header for 401 responses")
		require.Contains(t, wwwAuth, "Bearer", "WWW-Authenticate header should indicate Bearer authentication")
		require.Contains(t, wwwAuth, `realm="coder"`, "WWW-Authenticate header should include realm")

		t.Logf("Unauthenticated MCP access properly returned WWW-Authenticate: %s", wwwAuth)

		// Step 2: Perform dynamic client registration (RFC 7591)
		dynamicRegURL := api.AccessURL.String() + "/oauth2/register"

		// Create dynamic client registration request
		registrationRequest := map[string]any{
			"client_name":                "dynamic-mcp-client",
			"redirect_uris":              []string{"http://localhost:3000/callback"},
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
			"token_endpoint_auth_method": "client_secret_basic",
		}

		regBody, err := json.Marshal(registrationRequest)
		require.NoError(t, err)

		regReq, err := http.NewRequestWithContext(ctx, "POST", dynamicRegURL, strings.NewReader(string(regBody)))
		require.NoError(t, err)
		regReq.Header.Set("Content-Type", "application/json")

		// Dynamic client registration should not require authentication (public endpoint)
		regResp, err := client.Do(regReq)
		require.NoError(t, err)
		defer regResp.Body.Close()

		require.Equal(t, http.StatusCreated, regResp.StatusCode, "Dynamic client registration should succeed")

		// Parse the registration response
		var regResponse map[string]any
		err = json.NewDecoder(regResp.Body).Decode(&regResponse)
		require.NoError(t, err)

		clientID, ok := regResponse["client_id"].(string)
		require.True(t, ok, "Registration response should contain client_id")
		require.NotEmpty(t, clientID)

		clientSecret, ok := regResponse["client_secret"].(string)
		require.True(t, ok, "Registration response should contain client_secret")
		require.NotEmpty(t, clientSecret)

		t.Logf("Successfully registered dynamic client: %s", clientID)

		// Step 3: Perform OAuth2 authorization code flow with dynamically registered client
		authURL := fmt.Sprintf("%s/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=%s&state=dynamic_state",
			api.AccessURL.String(), clientID, "http://localhost:3000/callback")

		// Create an HTTP client that captures redirects
		authClient := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Stop following redirects
			},
		}

		// Make the authorization request with authentication
		authReq, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
		require.NoError(t, err)
		authReq.Header.Set("Cookie", fmt.Sprintf("coder_session_token=%s", coderClient.SessionToken()))

		authResp, err := authClient.Do(authReq)
		require.NoError(t, err)
		defer authResp.Body.Close()

		// Handle the response - check for error first
		if authResp.StatusCode == http.StatusBadRequest {
			// Read error response for debugging
			bodyBytes, err := io.ReadAll(authResp.Body)
			require.NoError(t, err)
			t.Logf("OAuth2 authorization error: %s", string(bodyBytes))
			t.FailNow()
		}

		// Handle consent flow if needed
		if authResp.StatusCode == http.StatusOK {
			// This means we got the consent page, now we need to POST consent
			consentReq, err := http.NewRequestWithContext(ctx, "POST", authURL, nil)
			require.NoError(t, err)
			consentReq.Header.Set("Cookie", fmt.Sprintf("coder_session_token=%s", coderClient.SessionToken()))
			consentReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			authResp, err = authClient.Do(consentReq)
			require.NoError(t, err)
			defer authResp.Body.Close()
		}

		// Extract authorization code from redirect
		require.True(t, authResp.StatusCode >= 300 && authResp.StatusCode < 400,
			"Expected redirect response, got %d", authResp.StatusCode)
		location := authResp.Header.Get("Location")
		require.NotEmpty(t, location, "Expected Location header in redirect")

		redirectURL, err := url.Parse(location)
		require.NoError(t, err)
		authCode := redirectURL.Query().Get("code")
		require.NotEmpty(t, authCode, "Expected authorization code in redirect URL")

		t.Logf("Successfully obtained authorization code: %s", authCode[:10]+"...")

		// Step 4: Exchange authorization code for access token
		tokenRequestBody := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"code":          {authCode},
			"redirect_uri":  {"http://localhost:3000/callback"},
		}

		tokenReq, err := http.NewRequestWithContext(ctx, "POST", api.AccessURL.String()+"/oauth2/tokens",
			strings.NewReader(tokenRequestBody.Encode()))
		require.NoError(t, err)
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		tokenResp, err := client.Do(tokenReq)
		require.NoError(t, err)
		defer tokenResp.Body.Close()

		require.Equal(t, http.StatusOK, tokenResp.StatusCode, "Token exchange should succeed")

		// Parse token response
		var tokenResponse map[string]any
		err = json.NewDecoder(tokenResp.Body).Decode(&tokenResponse)
		require.NoError(t, err)

		accessToken, ok := tokenResponse["access_token"].(string)
		require.True(t, ok, "Response should contain access_token")
		require.NotEmpty(t, accessToken)

		refreshToken, ok := tokenResponse["refresh_token"].(string)
		require.True(t, ok, "Response should contain refresh_token")
		require.NotEmpty(t, refreshToken)

		t.Logf("Successfully obtained access token: %s...", accessToken[:10])

		// Step 5: Use access token to get user information via MCP
		mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + accessToken,
			}))
		require.NoError(t, err)
		defer func() {
			if closeErr := mcpClient.Close(); closeErr != nil {
				t.Logf("Failed to close MCP client: %v", closeErr)
			}
		}()

		// Initialize MCP connection
		err = mcpClient.Start(ctx)
		require.NoError(t, err)

		initReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-dynamic-client",
					Version: "1.0.0",
				},
			},
		}

		result, err := mcpClient.Initialize(ctx, initReq)
		require.NoError(t, err)
		require.Equal(t, mcpserver.MCPServerName, result.ServerInfo.Name)

		// Get user information
		tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, tools.Tools)

		// Find and execute the authenticated user tool
		var userTool *mcp.Tool
		for _, tool := range tools.Tools {
			if tool.Name == toolsdk.ToolNameGetAuthenticatedUser {
				userTool = &tool
				break
			}
		}
		require.NotNil(t, userTool, "Expected to find "+toolsdk.ToolNameGetAuthenticatedUser+" tool")

		toolReq := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      userTool.Name,
				Arguments: map[string]any{},
			},
		}

		toolResult, err := mcpClient.CallTool(ctx, toolReq)
		require.NoError(t, err)
		require.NotEmpty(t, toolResult.Content)

		// Extract user info from first token
		var firstUserInfo string
		if textContent, ok := toolResult.Content[0].(mcp.TextContent); ok {
			firstUserInfo = textContent.Text
		} else {
			t.Errorf("Expected TextContent type, got %T", toolResult.Content[0])
		}
		require.NotEmpty(t, firstUserInfo)

		t.Logf("Successfully retrieved user info with first token")

		// Step 6: Refresh the token
		refreshRequestBody := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"refresh_token": {refreshToken},
		}

		refreshReq, err := http.NewRequestWithContext(ctx, "POST", api.AccessURL.String()+"/oauth2/tokens",
			strings.NewReader(refreshRequestBody.Encode()))
		require.NoError(t, err)
		refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		refreshResp, err := client.Do(refreshReq)
		require.NoError(t, err)
		defer refreshResp.Body.Close()

		require.Equal(t, http.StatusOK, refreshResp.StatusCode, "Token refresh should succeed")

		// Parse refresh response
		var refreshResponse map[string]any
		err = json.NewDecoder(refreshResp.Body).Decode(&refreshResponse)
		require.NoError(t, err)

		newAccessToken, ok := refreshResponse["access_token"].(string)
		require.True(t, ok, "Refresh response should contain new access_token")
		require.NotEmpty(t, newAccessToken)
		require.NotEqual(t, accessToken, newAccessToken, "New access token should be different")

		t.Logf("Successfully refreshed token: %s...", newAccessToken[:10])

		// Step 7: Use refreshed token to get user information again via MCP
		newMcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + newAccessToken,
			}))
		require.NoError(t, err)
		defer func() {
			if closeErr := newMcpClient.Close(); closeErr != nil {
				t.Logf("Failed to close new MCP client: %v", closeErr)
			}
		}()

		// Initialize new MCP connection
		err = newMcpClient.Start(ctx)
		require.NoError(t, err)

		newInitReq := mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "test-dynamic-client-refreshed",
					Version: "1.0.0",
				},
			},
		}

		newResult, err := newMcpClient.Initialize(ctx, newInitReq)
		require.NoError(t, err)
		require.Equal(t, mcpserver.MCPServerName, newResult.ServerInfo.Name)

		// Get user information with refreshed token
		newTools, err := newMcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, newTools.Tools)

		// Execute user tool again
		newToolResult, err := newMcpClient.CallTool(ctx, toolReq)
		require.NoError(t, err)
		require.NotEmpty(t, newToolResult.Content)

		// Extract user info from refreshed token
		var secondUserInfo string
		if textContent, ok := newToolResult.Content[0].(mcp.TextContent); ok {
			secondUserInfo = textContent.Text
		} else {
			t.Errorf("Expected TextContent type, got %T", newToolResult.Content[0])
		}
		require.NotEmpty(t, secondUserInfo)

		// Step 8: Compare user information before and after token refresh
		// Parse JSON to compare the important fields, ignoring timestamp differences
		var firstUser, secondUser map[string]any
		err = json.Unmarshal([]byte(firstUserInfo), &firstUser)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(secondUserInfo), &secondUser)
		require.NoError(t, err)

		// Compare key fields that should be identical
		require.Equal(t, firstUser["id"], secondUser["id"], "User ID should be identical")
		require.Equal(t, firstUser["username"], secondUser["username"], "Username should be identical")
		require.Equal(t, firstUser["email"], secondUser["email"], "Email should be identical")
		require.Equal(t, firstUser["status"], secondUser["status"], "Status should be identical")
		require.Equal(t, firstUser["login_type"], secondUser["login_type"], "Login type should be identical")
		require.Equal(t, firstUser["roles"], secondUser["roles"], "Roles should be identical")
		require.Equal(t, firstUser["organization_ids"], secondUser["organization_ids"], "Organization IDs should be identical")

		// Note: last_seen_at will be different since time passed between calls, which is expected

		t.Logf("Dynamic client registration flow test successful: " +
			"unauthenticated access → WWW-Authenticate → dynamic registration → OAuth2 flow → " +
			"MCP usage → token refresh → MCP usage with consistent user info")
	})

	// Test 6: Verify duplicate client names are allowed (RFC 7591 compliance)
	t.Run("DuplicateClientNamesAllowed", func(t *testing.T) {
		t.Parallel()

		dynamicRegURL := api.AccessURL.String() + "/oauth2/register"
		clientName := "duplicate-name-test-client"

		// Register first client with a specific name
		registrationRequest1 := map[string]any{
			"client_name":                clientName,
			"redirect_uris":              []string{"http://localhost:3000/callback1"},
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
			"token_endpoint_auth_method": "client_secret_basic",
		}

		regBody1, err := json.Marshal(registrationRequest1)
		require.NoError(t, err)

		regReq1, err := http.NewRequestWithContext(ctx, "POST", dynamicRegURL, strings.NewReader(string(regBody1)))
		require.NoError(t, err)
		regReq1.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		regResp1, err := client.Do(regReq1)
		require.NoError(t, err)
		defer regResp1.Body.Close()

		require.Equal(t, http.StatusCreated, regResp1.StatusCode, "First client registration should succeed")

		var regResponse1 map[string]any
		err = json.NewDecoder(regResp1.Body).Decode(&regResponse1)
		require.NoError(t, err)

		clientID1, ok := regResponse1["client_id"].(string)
		require.True(t, ok, "First registration response should contain client_id")
		require.NotEmpty(t, clientID1)

		// Register second client with the same name
		registrationRequest2 := map[string]any{
			"client_name":                clientName, // Same name as first client
			"redirect_uris":              []string{"http://localhost:3000/callback2"},
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
			"token_endpoint_auth_method": "client_secret_basic",
		}

		regBody2, err := json.Marshal(registrationRequest2)
		require.NoError(t, err)

		regReq2, err := http.NewRequestWithContext(ctx, "POST", dynamicRegURL, strings.NewReader(string(regBody2)))
		require.NoError(t, err)
		regReq2.Header.Set("Content-Type", "application/json")

		regResp2, err := client.Do(regReq2)
		require.NoError(t, err)
		defer regResp2.Body.Close()

		// This should succeed per RFC 7591 (no unique name requirement)
		require.Equal(t, http.StatusCreated, regResp2.StatusCode,
			"Second client registration with duplicate name should succeed (RFC 7591 compliance)")

		var regResponse2 map[string]any
		err = json.NewDecoder(regResp2.Body).Decode(&regResponse2)
		require.NoError(t, err)

		clientID2, ok := regResponse2["client_id"].(string)
		require.True(t, ok, "Second registration response should contain client_id")
		require.NotEmpty(t, clientID2)

		// Verify client IDs are different even though names are the same
		require.NotEqual(t, clientID1, clientID2, "Client IDs should be unique even with duplicate names")

		// Verify both clients have the same name but unique IDs
		name1, ok := regResponse1["client_name"].(string)
		require.True(t, ok)
		name2, ok := regResponse2["client_name"].(string)
		require.True(t, ok)

		require.Equal(t, clientName, name1, "First client should have the expected name")
		require.Equal(t, clientName, name2, "Second client should have the same name")
		require.Equal(t, name1, name2, "Both clients should have identical names")

		t.Logf("Successfully registered two OAuth2 clients with duplicate name '%s' but unique IDs: %s, %s",
			clientName, clientID1, clientID2)
	})
}

func TestMCPHTTP_E2E_ChatGPTEndpoint(t *testing.T) {
	// Setup Coder server with authentication
	coderClient, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	defer closer.Close()

	user := coderdtest.CreateFirstUser(t, coderClient)

	// Create template and workspace for testing search functionality
	version := coderdtest.CreateTemplateVersion(t, coderClient, user.OrganizationID, nil)
	awaitTemplateVersionJobCompleted(t, coderClient, version.ID)
	template := coderdtest.CreateTemplate(t, coderClient, user.OrganizationID, version.ID)

	// Create MCP client pointing to the ChatGPT endpoint
	mcpURL := api.AccessURL.String() + mcpserver.MCPEndpoint + "?toolset=chatgpt"

	// Configure client with authentication headers using RFC 6750 Bearer token
	mcpClient, err := mcpclient.NewStreamableHttpClient(mcpURL,
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + coderClient.SessionToken(),
		}))
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := mcpClient.Close(); closeErr != nil {
			t.Logf("Failed to close MCP client: %v", closeErr)
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()

	// Start client
	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	// Initialize connection
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-chatgpt-client",
				Version: "1.0.0",
			},
		},
	}

	result, err := mcpClient.Initialize(ctx, initReq)
	require.NoError(t, err)
	require.Equal(t, mcpserver.MCPServerName, result.ServerInfo.Name)
	require.Equal(t, mcp.LATEST_PROTOCOL_VERSION, result.ProtocolVersion)
	require.NotNil(t, result.Capabilities)

	// Test tool listing - should only have search and fetch tools for ChatGPT
	tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, tools.Tools)

	// Verify we have exactly the ChatGPT tools and no others
	var foundTools []string
	for _, tool := range tools.Tools {
		foundTools = append(foundTools, tool.Name)
	}

	// ChatGPT endpoint should only expose search and fetch tools
	assert.Contains(t, foundTools, toolsdk.ToolNameChatGPTSearch, "Should have ChatGPT search tool")
	assert.Contains(t, foundTools, toolsdk.ToolNameChatGPTFetch, "Should have ChatGPT fetch tool")
	assert.Len(t, foundTools, 2, "ChatGPT endpoint should only expose search and fetch tools")

	// Should NOT have other tools that are available in the standard endpoint
	assert.NotContains(t, foundTools, toolsdk.ToolNameGetAuthenticatedUser, "Should not have authenticated user tool")
	assert.NotContains(t, foundTools, toolsdk.ToolNameListWorkspaces, "Should not have list workspaces tool")

	t.Logf("ChatGPT endpoint tools: %v", foundTools)

	// Test search tool - search for templates
	var searchTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == toolsdk.ToolNameChatGPTSearch {
			searchTool = &tool
			break
		}
	}
	require.NotNil(t, searchTool, "Expected to find search tool")

	// Execute search for templates
	searchReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: searchTool.Name,
			Arguments: map[string]any{
				"query": "templates",
			},
		},
	}

	searchResult, err := mcpClient.CallTool(ctx, searchReq)
	require.NoError(t, err)
	require.NotEmpty(t, searchResult.Content)

	// Verify the search result contains our template
	assert.Len(t, searchResult.Content, 1)
	if textContent, ok := searchResult.Content[0].(mcp.TextContent); ok {
		assert.Equal(t, "text", textContent.Type)
		assert.Contains(t, textContent.Text, template.ID.String(), "Search result should contain our test template")
		t.Logf("Search result: %s", textContent.Text)
	} else {
		t.Errorf("Expected TextContent type, got %T", searchResult.Content[0])
	}

	// Test fetch tool
	var fetchTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == toolsdk.ToolNameChatGPTFetch {
			fetchTool = &tool
			break
		}
	}
	require.NotNil(t, fetchTool, "Expected to find fetch tool")

	// Execute fetch for the template
	fetchReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: fetchTool.Name,
			Arguments: map[string]any{
				"id": fmt.Sprintf("template:%s", template.ID.String()),
			},
		},
	}

	fetchResult, err := mcpClient.CallTool(ctx, fetchReq)
	require.NoError(t, err)
	require.NotEmpty(t, fetchResult.Content)

	// Verify the fetch result contains template details
	assert.Len(t, fetchResult.Content, 1)
	if textContent, ok := fetchResult.Content[0].(mcp.TextContent); ok {
		assert.Equal(t, "text", textContent.Type)
		assert.Contains(t, textContent.Text, template.Name, "Fetch result should contain template name")
		assert.Contains(t, textContent.Text, template.ID.String(), "Fetch result should contain template ID")
		t.Logf("Fetch result contains template data")
	} else {
		t.Errorf("Expected TextContent type, got %T", fetchResult.Content[0])
	}

	t.Logf("ChatGPT endpoint E2E test successful: search and fetch tools working correctly")
}

// awaitTemplateVersionJobCompleted waits for the template version provisioner job
// to complete. CI environments can be slower and more contended, so we use a
// longer timeout when CI is detected.
func awaitTemplateVersionJobCompleted(t testing.TB, client *codersdk.Client, versionID uuid.UUID) codersdk.TemplateVersion {
	t.Helper()

	timeout := testutil.WaitLong
	if testutil.InCI() {
		timeout = testutil.WaitSuperLong
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	t.Logf("waiting for template version %s build job to complete (timeout: %s)", versionID.String(), timeout)

	var (
		templateVersion codersdk.TemplateVersion
		lastStatus      codersdk.ProvisionerJobStatus
		lastErr         error
	)

	require.Eventually(t, func() bool {
		var err error
		templateVersion, err = client.TemplateVersion(ctx, versionID)
		if err != nil {
			lastErr = err
			return false
		}
		lastErr = nil

		if templateVersion.Job.Status != lastStatus {
			lastStatus = templateVersion.Job.Status
			t.Logf("template version job status: %s", templateVersion.Job.Status)
		}

		return templateVersion.Job.CompletedAt != nil
	}, timeout, testutil.IntervalMedium,
		"template version %s build job did not complete within %s (lastStatus=%s lastErr=%v); make sure you set `IncludeProvisionerDaemon`!",
		versionID.String(), timeout, lastStatus, lastErr,
	)

	t.Logf("template version %s job has completed", versionID.String())
	return templateVersion
}

// Helper function to parse URL safely in tests
func mustParseURL(t *testing.T, rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	require.NoError(t, err, "Failed to parse URL %q", rawURL)
	return u
}
