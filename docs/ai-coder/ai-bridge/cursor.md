# Cursor with AI Bridge

[Cursor](https://cursor.sh/) is an AI-first code editor built on VS Code.

## Support Status

- **OpenAI**: ⚠️ Partial Support (limited models, proxied through Cursor servers)
- **Anthropic**: ❌ Not Supported (no base URL configuration option)

## Prerequisites

- Cursor IDE installed
- Coder session token
- Cursor Pro subscription (recommended)

## Configuration

### Step 1: Open Cursor Settings

1. Launch Cursor
2. Open Settings (File → Preferences → Settings or `Cmd/Ctrl + ,`)
3. Navigate to AI settings section

### Step 2: Configure OpenAI Base URL

1. Find the **OpenAI Base URL** setting
2. Set to: `https://coder.example.com/api/experimental/aibridge/openai/v1`
3. Set **API Key** to your Coder session token

### Step 3: Set Environment Variables (Alternative)

You can also configure via environment variables:

```sh
export OPENAI_BASE_URL="https://coder.example.com/api/experimental/aibridge/openai/v1"
export OPENAI_API_KEY="your-coder-session-token"
```

Then launch Cursor from the terminal where these are set.

## Template Configuration

Pre-configure in your Coder template:

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    OPENAI_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/openai/v1"
    OPENAI_API_KEY  = data.coder_workspace_owner.me.session_token
  }
}
```

## Known Limitations

### Limited Model Support

When using a custom API endpoint, Cursor only supports **`gpt-4.1`** model. Other GPT models are disabled by Cursor when a custom base URL is configured.

### Requests Proxied Through Cursor Servers

Even with a custom base URL, all requests are still **proxied through Cursor's servers**. This means:

- Cursor can see your requests
- Adds additional latency
- May not meet all compliance requirements

### No Anthropic Support

Cursor does not provide an option to configure the Anthropic base URL. You cannot use:

- Claude models
- Anthropic API features

If you need Anthropic support, consider alternative clients:

- [Claude Code](./claude-code.md)
- [Roo Code](./roo-code.md)

### No Centralized Configuration

There is no way to centrally configure the custom endpoint for all users in your organization. Each user must configure their own Cursor instance.

## Troubleshooting

### AI Features Not Working

1. Verify the base URL is set correctly in settings
2. Check your API key is a valid Coder session token
3. Ensure AI Bridge is enabled on the server
4. Try restarting Cursor completely

### Models Not Available

Only `gpt-4.1` is available with custom endpoints. This is a Cursor limitation, not an AI Bridge limitation.

### Authentication Errors

```sh
# Generate a fresh Coder token
coder tokens create

# Verify it works
curl -H "Coder-Session-Token: YOUR_TOKEN" \
  https://coder.example.com/api/v2/users/me
```

### Compliance Concerns

If Cursor's proxy behavior doesn't meet your compliance requirements, consider these alternatives:

- [Claude Code](./claude-code.md) - Direct connection, full Anthropic support
- [Roo Code](./roo-code.md) - Direct connection, OpenAI + Anthropic
- [GitHub Copilot](./github-copilot.md) - Enterprise-grade with fine-grained controls

## Alternatives

If Cursor's limitations are blocking for you, these alternatives may work better:

| Client                                    | OpenAI | Anthropic | Proxy-Free | Centralized Config |
|-------------------------------------------|--------|-----------|------------|--------------------|
| **[Claude Code](./claude-code.md)**       | ❌      | ✅         | ✅          | ✅                  |
| **[Roo Code](./roo-code.md)**             | ⚠️     | ✅         | ✅          | ✅                  |
| **[GitHub Copilot](./github-copilot.md)** | ✅      | ❌         | ❌          | ⚠️                 |
| **Cursor**                                | ⚠️     | ❌         | ❌          | ❌                  |

## Related Documentation

- [AI Bridge Setup](./index.md#setup)
- [Client Configuration Overview](./index.md#client-configuration)
- [Other Supported Clients](./index.md#supported-clients)
