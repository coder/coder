# OAuth2 Test Scripts

This directory contains test scripts for the MCP OAuth2 implementation in Coder.

## Prerequisites

1. Start Coder in development mode:

   ```bash
   ./scripts/develop.sh
   ```

2. Login to get a session token:

   ```bash
   ./scripts/coder-dev.sh login
   ```

## Scripts

### `test-mcp-oauth2.sh`

Complete automated test suite that verifies all OAuth2 functionality:

- Metadata endpoint
- PKCE flow
- Resource parameter support
- Token refresh
- Error handling

Usage:

```bash
chmod +x ./scripts/oauth2/test-mcp-oauth2.sh
./scripts/oauth2/test-mcp-oauth2.sh
```

### `setup-test-app.sh`

Creates a test OAuth2 application and outputs environment variables.

Usage:

```bash
eval $(./scripts/oauth2/setup-test-app.sh)
echo "Client ID: $CLIENT_ID"
```

### `cleanup-test-app.sh`

Deletes a test OAuth2 application.

Usage:

```bash
./scripts/oauth2/cleanup-test-app.sh $CLIENT_ID
# Or if CLIENT_ID is set as environment variable:
./scripts/oauth2/cleanup-test-app.sh
```

### `generate-pkce.sh`

Generates PKCE code verifier and challenge for manual testing.

Usage:

```bash
./scripts/oauth2/generate-pkce.sh
```

### `test-manual-flow.sh`

Launches a local Go web server to test the OAuth2 flow interactively. The server automatically handles the OAuth2 callback and token exchange, providing a user-friendly web interface with results.

Usage:

```bash
# First set up an app
eval $(./scripts/oauth2/setup-test-app.sh)

# Then run the test server
./scripts/oauth2/test-manual-flow.sh
```

Features:

- Starts a local web server on port 9876
- Automatically captures the authorization code
- Performs token exchange without manual intervention
- Displays results in a clean web interface
- Shows example API calls you can make with the token

### `oauth2-test-server.go`

A Go web server that handles OAuth2 callbacks and token exchange. Used internally by `test-manual-flow.sh` but can also be run standalone:

```bash
export CLIENT_ID="your-client-id"
export CLIENT_SECRET="your-client-secret"
export CODE_VERIFIER="your-code-verifier"
export STATE="your-state"
go run ./scripts/oauth2/oauth2-test-server.go
```

## Example Workflow

1. **Run automated tests:**

   ```bash
   ./scripts/oauth2/test-mcp-oauth2.sh
   ```

2. **Interactive browser testing:**

   ```bash
   # Create app
   eval $(./scripts/oauth2/setup-test-app.sh)

   # Run the test server (opens in browser automatically)
   ./scripts/oauth2/test-manual-flow.sh
   # - Opens authorization URL in terminal
   # - Handles callback automatically
   # - Shows token exchange results

   # Clean up when done
   ./scripts/oauth2/cleanup-test-app.sh
   ```

3. **Generate PKCE for custom testing:**

   ```bash
   ./scripts/oauth2/generate-pkce.sh
   # Use the generated values in your own curl commands
   ```

## Environment Variables

All scripts respect these environment variables:

- `SESSION_TOKEN`: Coder session token (auto-read from `.coderv2/session`)
- `BASE_URL`: Coder server URL (default: `http://localhost:3000`)
- `CLIENT_ID`: OAuth2 client ID
- `CLIENT_SECRET`: OAuth2 client secret

## OAuth2 Endpoints

- Metadata: `GET /.well-known/oauth-authorization-server`
- Authorization: `GET/POST /oauth2/authorize`
- Token: `POST /oauth2/tokens`
- Apps API: `/api/v2/oauth2-provider/apps`
