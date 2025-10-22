# Roo Code with AI Bridge

[Roo Code](https://github.com/RooVetGit/Roo-Code) is an AI coding assistant extension for VS Code.

## Support Status

- **OpenAI**: ⚠️ Partial Support (needs `/v1`, use legacy API format)
- **Anthropic**: ✅ Fully Supported

## Prerequisites

- Visual Studio Code
- Roo Code extension
- Coder session token

## Configuration

### Step 1: Install Roo Code

Install the Roo Code extension from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=RooVeterinaryInc.roo-coder).

### Step 2: Configure Settings

1. Open VS Code Settings (`Cmd/Ctrl + ,`)
2. Search for "Roo Code"
3. Set **Provider Type** to **"OpenAI Compatible"**
4. Enable **"Use legacy OpenAI API format"**

### Step 3: Set Environment Variables

Configure your shell environment:

```sh
# OpenAI configuration
export OPENAI_BASE_URL="https://coder.example.com/api/experimental/aibridge/openai/v1"
export OPENAI_API_KEY="your-coder-session-token"

# Anthropic configuration
export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"
```

### Step 4: Restart VS Code

Close and reopen VS Code for the environment variables to take effect.

## Template Configuration

Pre-configure Roo Code in your Coder template:

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    # OpenAI configuration
    OPENAI_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/openai/v1"
    OPENAI_API_KEY  = data.coder_workspace_owner.me.session_token

    # Anthropic configuration
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
  }
}
```

## Known Limitations

### OpenAI /v1/responses API Not Supported

Roo Code uses the newer OpenAI `/v1/responses` API for some features. AI Bridge does not yet support this endpoint.

**Workaround**: Use the legacy OpenAI API format (configured in Step 2 above).

### MCP Tools

Some MCP tools (like `star_github_repository`) may appear as available but fail with permission errors. This is typically due to:

- Missing OAuth authentication for the external service
- Insufficient permissions in the MCP server configuration

## Troubleshooting

### Extension Not Detecting AI Bridge

1. Verify environment variables are set:

   ```sh
   echo $OPENAI_BASE_URL
   echo $ANTHROPIC_BASE_URL
   ```

2. Ensure "OpenAI Compatible" provider is selected in settings

3. Restart VS Code completely (not just reload window)

### Authentication Errors

```sh
# Generate a fresh Coder token
coder tokens create

# Verify token works
curl -H "Coder-Session-Token: YOUR_TOKEN" \
  https://coder.example.com/api/v2/users/me
```

### Models Not Available

Ensure you've enabled the legacy API format in Roo Code settings.

## Related Documentation

- [AI Bridge Setup](./index.md#setup)
- [Client Configuration Overview](./index.md#client-configuration)
- [Other Supported Clients](./index.md#supported-clients)
- [Kilo Code](./other-clients.md#kilo-code) (Roo Code fork)
