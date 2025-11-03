# Client Configuration

Once AI Bridge is enabled on the server, your users need to configure their AI coding tools to use it. This section explains how users should configure their clients to connect to AI Bridge.

### Base URLs

The exact configuration method varies by client — some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**: Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/openai/v1`
- **Anthropic-compatible clients**: Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

### Authentication

Instead of distributing provider-specific API keys (OpenAI/Anthropic keys) to users, they authenticate to AI Bridge using their **Coder session token** or **API key**:

- **OpenAI clients**: Users set `OPENAI_API_KEY` to their Coder session token or API key
- **Anthropic clients**: Users set `ANTHROPIC_API_KEY` to their Coder session token or API key

#### Coder Templates Pre-configuration

Template admins can pre-configure authentication in templates using [`data.coder_workspace_owner.me.session_token`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner#session_token-1) to automatically configure the workspace owner's credentials.

Here is an example of how to pre-configure a Coder template to install Claude Code and configure it for AI Bridge using the session token in a template:

```hcl
data "coder_workspace_owner" "me" {}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    ANTHROPIC_BASE_URL : "https://dev.coder.com/api/v2/aibridge/anthropic",
    ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
  }
  ... # other agent configuration
}

# See https://registry.coder.com/modules/coder/claude-code for more information
module "claude-code" {
  count               = local.has_ai_prompt ? data.coder_workspace.me.start_count : 0
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = ">= 3.2.0"
  agent_id            = coder_agent.dev.id
  workdir             = "/home/coder/project"
  order               = 999
  claude_api_key      = data.coder_workspace_owner.me.session_token # To Enable AI Bridge integration
  ai_prompt           = data.coder_parameter.ai_prompt.value
  ... # other claude-code configuration
}

```

The same approach can be applied to pre-configure additional AI coding assistants by updating the base URL and API key settings.

#### Generic API key generation

Users can generate a Coder API key using either the CLI or the web UI. Follow the instructions at [Sessions and API tokens](../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) to generate a Coder API key.

### Tested clients

The combinations below reflect what we have exercised so far. Use the upstream links for vendor-specific steps to point each client at Bridge. Share additional findings in the [`aibridge`](https://github.com/coder/aibridge) issue tracker so we can keep this table current.

| Client                                                                                                                                    | OpenAI support | Anthropic support | Notes                                                                                                                                                                            |
|-------------------------------------------------------------------------------------------------------------------------------------------|----------------|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Claude Code](https://docs.claude.com/en/docs/claude-code/settings#environment-variables)                                                 | N/A            | ✅                 | Works out of the box and can be preconfigured in templates.                                                                                                                      |
| Claude Code (VS Code)                                                                                                                     | N/A            | ✅                 | May require signing in once; afterwards respects workspace environment variables.                                                                                                |
| [Cursor](https://cursor.com/docs/settings/api-keys)                                                                                       | ⚠️             | ❌                 | Only non reasoning models like `gpt-4.1` are available when using a custom endpoint. Requests still transit Cursor's cloud. There is no central admin setting to configure this. |
| [Roo Code](https://docs.roocode.com/features/api-configuration-profiles#creating-and-managing-profiles)                                   | ✅              | ✅                 | Use the **OpenAI Compatible** provider with the legacy format to avoid `/v1/responses`.                                                                                          |
| [Codex CLI](https://github.com/openai/codex/blob/main/docs/config.md#model_providers)                                                     | ✅              | N/A               | `gpt-5-codex` support is [in progress](https://github.com/coder/aibridge/issues/16).                                                                                             |
| [GitHub Copilot (VS Code)](https://docs.github.com/en/copilot/configuring-github-copilot/configuring-network-settings-for-github-copilot) | ✅              | ❌                 | Requires the pre-release extension. Anthropic endpoints are not supported.                                                                                                       |
| Goose                                                                                                                                     | ❓              | ❓                 |                                                                                                                                                                                  |
| Goose Desktop                                                                                                                             | ❓              | ✅                 |                                                                                                                                                                                  |
| WindSurf                                                                                                                                  | ❌              | —                 | No option to override the base URL.                                                                                                                                              |
| Sourcegraph Amp                                                                                                                           | ❌              | —                 | No option to override the base URL.                                                                                                                                              |
| Kiro                                                                                                                                      | ❌              | —                 | No option to override the base URL.                                                                                                                                              |
| [Copilot CLI](https://github.com/github/copilot-cli/issues/104)                                                                           | ❌              | ❌                 | No support for custom base URLs and uses a `GITHUB_TOKEN` for authentication.                                                                                                    |
| [Kilo Code](https://kilocode.ai/docs/features/api-configuration-profiles#creating-and-managing-profiles)                                  | ✅              | ✅                 | Similar to Roo Code.                                                                                                                                                             |
| Gemini CLI                                                                                                                                | ❌              | ❌                 | Not supported yet (`GOOGLE_GEMINI_BASE_URL`).                                                                                                                                    |
| [Amazon Q CLI](https://aws.amazon.com/q/)                                                                                                 | ❌              | ❌                 | Limited to Amazon Q subscriptions; no custom endpoint support.                                                                                                                   |

Legend: ✅ works, ⚠️ limited support, ❌ not supported, ❓ not yet verified, — not applicable.

> [!NOTE]
> Click the respective client title to view the vendor-specific instructions for configuring the client.

#### Compatibility overview

Most AI coding assistants that support custom base URLs can work with AI Bridge. Client-specific requirements vary:

- Some clients require specific URL formats (for example, removing the `/v1` suffix).
- Some clients proxy requests through their own servers, which limits compatibility.
- Some clients do not support custom base URLs.

See the [tested clients](#tested-clients) table above for the combinations we have verified and any known issues.
