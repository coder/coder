# Client Configuration

Once AI Bridge is setup on your deployment, the AI coding tools used by your users will need to be configured to route requests via AI Bridge.

## Base URLs

Most AI coding tools allow the "base URL" to be customized. In other words, when a request is made to OpenAI's API from your coding tool, the API endpoint such as [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat) will be appended to the configured base. Therefore, instead of the default base URL of `https://api.openai.com/v1`, you'll need to set it to `https://coder.example.com/api/v2/aibridge/openai/v1`.

The exact configuration method varies by client — some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**: Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/openai/v1`
- **Anthropic-compatible clients**: Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

## Authentication

Instead of distributing provider-specific API keys (OpenAI/Anthropic keys) to users, they authenticate to AI Bridge using their **Coder session token** or **API key**:

- **OpenAI clients**: Users set `OPENAI_API_KEY` to their Coder session token or API key
- **Anthropic clients**: Users set `ANTHROPIC_API_KEY` to their Coder session token or API key

> [!NOTE]
> Only Coder-issued tokens can authenticate users against AI Bridge.
> AI Bridge will use provider-specific API keys to [authenticate against upstream AI services](https://coder.com/docs/ai-coder/ai-bridge/setup#configure-providers).

Again, the exact environment variable or setting naming may differ from tool to tool. See a list of [supported clients](#all-supported-clients) below and consult your tool's documentation for details.

### Retrieving your session token

[Generate a long-lived API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) via the Coder dashboard and use it to configure your AI coding tool:

```sh
export ANTHROPIC_API_KEY="your-coder-session-token"
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/aibridge/anthropic"
```

## Compatibility

The table below shows tested AI clients and their compatibility with AI Bridge.

| Client                           | OpenAI | Anthropic | Notes                                                                                                                                                  |
|----------------------------------|--------|-----------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Claude Code](./claude-code.md)  | -      | ✅         |                                                                                                                                                        |
| [Codex CLI](./codex.md)          | ✅      | -         |                                                                                                                                                        |
| [OpenCode](./opencode.md)        | ✅      | ✅         |                                                                                                                                                        |
| [Factory](./factory.md)          | ✅      | ✅         |                                                                                                                                                        |
| [Cline](./cline.md)              | ✅      | ✅         |                                                                                                                                                        |
| [Kilo Code](./kilo-code.md)      | ✅      | ✅         |                                                                                                                                                        |
| [Roo Code](./roo-code.md)        | ✅      | ✅         |                                                                                                                                                        |
| [VS Code](./vscode.md)           | ✅      | ❌         | Only supports Custom Base URL for OpenAI.                                                                                                              |
| [JetBrains IDEs](./jetbrains.md) | ✅      | ❌         | Works in Chat mode via "Bring Your Own Key".                                                                                                           |
| [Zed](./zed.md)                  | ✅      | ✅         |                                                                                                                                                        |
| WindSurf                         | ❌      | ❌         | No option to override base URL.                                                                                                                        |
| Cursor                           | ❌      | ❌         | Override for OpenAI broken ([upstream issue](https://forum.cursor.com/t/requests-are-sent-to-incorrect-endpoint-when-using-base-url-override/144894)). |
| Sourcegraph Amp                  | ❌      | ❌         | No option to override base URL.                                                                                                                        |
| Kiro                             | ❌      | ❌         | No option to override base URL.                                                                                                                        |
| Gemini CLI                       | ❌      | ❌         | No Gemini API support. Upvote [this issue](https://github.com/coder/aibridge/issues/27).                                                               |
| Antigravity                      | ❌      | ❌         | No option to override base URL.                                                                                                                        |
|

*Legend: ✅ supported, ❌ not supported, - not applicable.*

## Configuring In-Workspace Tools

AI coding tools running inside a Coder workspace, such as IDE extensions, can be configured to use AI Bridge.

While users can manually configure these tools with a long-lived API key, template admins can provide a more seamless experience by pre-configuring them. Admins can automatically inject the user's session token with `data.coder_workspace_owner.me.session_token` and the AI Bridge base URL into the workspace environment.

In this example, Claude Code respects these environment variables and will route all requests via AI Bridge.

```hcl
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
    arch = "amd64"
    os   = "linux"
    dir  = local.repo_dir
    env = {
        ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic",
        ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
    }
    ... # other agent configuration
}
```

## External and Desktop Clients

You can also configure AI tools running outside of a Coder workspace, such as local IDE extensions or desktop applications, to connect to AI Bridge.

The configuration is the same: point the tool to the AI Bridge [base URL](#base-urls) and use a Coder API key for authentication.

Users can generate a long-lived API key from the Coder UI or CLI. Follow the instructions at [Sessions and API tokens](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) to create one.

## All Supported Clients

<children></children>
