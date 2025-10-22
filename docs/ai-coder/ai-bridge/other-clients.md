# Other Clients with AI Bridge

This page covers AI coding assistants with AI Bridge support that are less commonly used or are forks of other tools.

## Kilo Code

[Kilo Code](https://github.com/Open-Code-Coop/KiloCode) is a fork of Roo Code with similar functionality.

### Support Status

- **OpenAI**: ⚠️ Partial Support (use legacy API format)
- **Anthropic**: ✅ Fully Supported

### Configuration

Kilo Code configuration is nearly identical to [Roo Code](./roo-code.md):

1. Open VS Code Settings (`Cmd/Ctrl + ,`)
1. Search for "Kilo Code"
1. Switch to **legacy OpenAI format**
1. Set environment variables:

```sh
export OPENAI_BASE_URL="https://coder.example.com/api/experimental/aibridge/openai/v1"
export OPENAI_API_KEY="your-coder-session-token"

export ANTHROPIC_BASE_URL="https://coder.example.com/api/experimental/aibridge/anthropic"
export ANTHROPIC_API_KEY="your-coder-session-token"
```

1. Restart VS Code

### Template Configuration

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    OPENAI_BASE_URL    = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/openai/v1"
    OPENAI_API_KEY     = data.coder_workspace_owner.me.session_token
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/experimental/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token
  }
}
```

### Differences from Roo Code

- Different branding and UI
- May have slightly different feature sets
- Same underlying configuration approach

For detailed troubleshooting, see the [Roo Code documentation](./roo-code.md#troubleshooting).

## Unsupported Clients

The following clients do not currently support custom base URLs and **cannot** be used with AI Bridge:

### WindSurf

**Status**: ❌ Not Supported

[WindSurf](https://codeium.com/windsurf) by Codeium does not provide an option to override the base URL for AI API calls.

**Alternative**: If you need Codeium features with AI Bridge, contact Codeium support to request base URL configuration options.

### Sourcegraph Cody

**Status**: ❌ Not Supported

[Sourcegraph Cody](https://sourcegraph.com/cody) does not support custom base URL configuration.

**Alternative**: Use [Claude Code](./claude-code.md) or [Roo Code](./roo-code.md) for similar functionality with AI Bridge support.

### Kiro

**Status**: ❌ Not Supported

[Kiro](https://kiro.ai/) does not provide base URL override options.

### Copilot CLI

**Status**: ❌ Not Supported

[GitHub Copilot CLI](https://github.com/github/copilot-cli) uses GitHub token authentication exclusively and does not support custom base URLs.

**Tracking Issue**: [github/copilot-cli#104](https://github.com/github/copilot-cli/issues/104)

**Alternative**: Use [Claude Code CLI](./claude-code.md#claude-code-cli) or [Goose CLI](./goose.md#goose-cli) for CLI-based AI assistance with AI Bridge.

## Requesting Support for Your Client

If your preferred AI coding assistant isn't listed here, you can:

1. **Check with the vendor**: Ask if they support custom base URLs
2. **File a feature request**: Request base URL configuration in their issue tracker
3. **Submit to Coder**: Let us know which clients you'd like to see tested with AI Bridge

### What Makes a Client Compatible?

For a client to work with AI Bridge, it must support:

- ✅ Custom base URL configuration (via settings or environment variables)
- ✅ Custom API key configuration
- ✅ Standard OpenAI or Anthropic API formats

## Related Documentation

- [Supported Clients Overview](./index.md#supported-clients)
- [Roo Code](./roo-code.md) - Detailed configuration for Kilo Code's parent project
- [Claude Code](./claude-code.md)
- [AI Bridge Setup](./index.md#setup)
