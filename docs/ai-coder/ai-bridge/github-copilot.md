# GitHub Copilot with AI Bridge

[GitHub Copilot](https://github.com/features/copilot) is GitHub's AI-powered code completion tool.

## Support Status

| Variant                       | AI Bridge Support | Notes                                             |
|-------------------------------|-------------------|---------------------------------------------------|
| **GitHub Copilot in VS Code** | ✅ Supported       | Requires VS Code Insiders                         |
| **GitHub Copilot CLI**        | ❌ Not Supported   | Uses GitHub token auth only, no base URL override |

## GitHub Copilot in VS Code

### Prerequisites

- [VS Code Insiders](https://code.visualstudio.com/insiders/)
- GitHub Copilot pre-release extension
- Coder session token

### Configuration

1. Install GitHub Copilot pre-release extension in VS Code Insiders
1. Configure via environment variables:

```sh
export OPENAI_BASE_URL="https://coder.example.com/api/experimental/aibridge/openai/v1"
export OPENAI_API_KEY="your-coder-session-token"
```

1. Restart VS Code Insiders

### Limitations

- Only OpenAI models are supported (no Anthropic)
- Requires pre-release version of the extension
- Standard GitHub Copilot subscription required

### Template Configuration

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

## GitHub Copilot CLI

### Why It's Not Supported

The GitHub Copilot CLI:

- Authenticates exclusively via `GH_TOKEN` (GitHub personal access token)
- Does not support custom base URL configuration
- Has a fixed set of models tied to GitHub's backend

**Tracking Issue**: [github/copilot-cli#104](https://github.com/github/copilot-cli/issues/104)

If you need CLI-based AI assistance with AI Bridge, consider:

- [Claude Code CLI](./claude-code.md#claude-code-cli)
- [Goose CLI](./goose.md#goose-cli)

## Troubleshooting

### Extension Not Working

1. Verify you're using VS Code Insiders (not stable)
2. Ensure pre-release extension is installed
3. Check environment variables are set correctly
4. Restart VS Code completely

### Token Issues

```sh
# Verify your Coder session token
coder tokens create

# Test the token
curl -H "Coder-Session-Token: YOUR_TOKEN" \
  https://coder.example.com/api/v2/users/me
```

## Related Documentation

- [AI Bridge Setup](./index.md#setup)
- [Client Configuration Overview](./index.md#client-configuration)
- [Other Supported Clients](./index.md#supported-clients)
