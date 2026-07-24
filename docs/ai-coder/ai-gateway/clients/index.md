# Client Configuration

> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../../ai-governance.md).
> As of Coder v2.32, deployments without the add-on will not be able to
> access AI Gateway.

Once AI Gateway is setup on your deployment, the AI coding tools used by your users will need to be configured to route requests via AI Gateway.

There are two ways to connect AI tools to AI Gateway:

- Base URL configuration (Recommended): Most AI tools allow customizing the base URL for API requests. This is the preferred approach when supported.
- AI Gateway Proxy: For tools that don't support base URL configuration, [AI Gateway Proxy](../ai-gateway-proxy/index.md) can intercept traffic and forward it to AI Gateway.

> [!NOTE]
> AI Gateway works with tools running inside or outside of Coder workspaces.
> For non-workspace setup, visit [External and Desktop Clients](#external-and-desktop-clients).

## Base URLs

Most AI coding tools allow the "base URL" to be customized. In other words, when a request is made to OpenAI's API from your coding tool, the API endpoint such as [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat) will be appended to the configured base. Therefore, instead of the default base URL of `https://api.openai.com/v1`, you'll need to set it to `https://coder.example.com/api/v2/ai-gateway/openai/v1`.

The exact configuration method varies by client, some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**: Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/ai-gateway/openai/v1`
- **Anthropic-compatible clients**: Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/ai-gateway/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

## Authentication

For information about authenticating with AI Gateway, visit [AI Gateway Authentication](../auth.md).

## Compatibility

The table below shows tested AI clients and their compatibility with AI Gateway.

| Client                           | OpenAI | Anthropic | BYOK | Notes                                                                                                                                                  |
|----------------------------------|--------|-----------|------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Mux](./mux.md)                  | ✅      | ✅         | -    |                                                                                                                                                        |
| [Claude Code](./claude-code.md)  | -      | ✅         | ✅    |                                                                                                                                                        |
| [Codex CLI](./codex.md)          | ✅      | -         | ✅    |                                                                                                                                                        |
| [OpenCode](./opencode.md)        | ✅      | ✅         | ✅    |                                                                                                                                                        |
| [Factory](./factory.md)          | ✅      | ✅         | ✅    |                                                                                                                                                        |
| [Cline](./cline.md)              | ✅      | ✅         | ✅    |                                                                                                                                                        |
| [Kilo Code](./kilo-code.md)      | ✅      | ✅         | ❌    |                                                                                                                                                        |
| [VS Code](./vscode.md)           | ✅      | ✅         | ❌    | VS Code 1.122+ via Custom Endpoint provider. GitHub sign-in not required. Inline suggestions still require GitHub Copilot.                             |
| [JetBrains IDEs](./jetbrains.md) | ✅      | ❌         | ❌    | Works in Chat mode via [third-party model configuration](https://www.jetbrains.com/help/ai-assistant/use-custom-models.html#provide-your-own-api-key). |
| [Zed](./zed.md)                  | ✅      | ✅         | ❌    |                                                                                                                                                        |
| [GitHub Copilot](./copilot.md)   | ⚙️     | -         | -    | Requires [AI Gateway Proxy](../ai-gateway-proxy/index.md). Uses per-user GitHub tokens.                                                                |
| WindSurf                         | ❌      | ❌         | ❌    | No option to override base URL.                                                                                                                        |
| Cursor                           | ❌      | ❌         | ❌    | Override for OpenAI broken ([upstream issue](https://forum.cursor.com/t/requests-are-sent-to-incorrect-endpoint-when-using-base-url-override/144894)). |
| Sourcegraph Amp                  | ❌      | ❌         | ❌    | No option to override base URL.                                                                                                                        |
| Kiro                             | ❌      | ❌         | ❌    | No option to override base URL.                                                                                                                        |
| Gemini CLI                       | ❌      | ❌         | ❌    | No Gemini API support. Upvote [this issue](https://github.com/coder/coder/issues/24804).                                                               |
| Antigravity                      | ❌      | ❌         | ❌    | No option to override base URL.                                                                                                                        |
|

*Legend: ✅ supported, ⚙️ requires AI Gateway Proxy, ❌ not supported, - not applicable.*

## Configuring In-Workspace Tools

AI coding tools running inside a Coder workspace, such as IDE extensions, can be configured to use AI Gateway.

This section applies when you want template admins to preconfigure tools inside Coder workspaces. For tools running outside of a workspace, see [External and Desktop Clients](#external-and-desktop-clients).

While users can manually configure these tools with a long-lived API key, template admins can provide a more seamless experience by pre-configuring them. Admins can automatically inject the user's session token with `data.coder_workspace_owner.me.session_token` and the AI Gateway base URL into the workspace environment.

In this example, Claude Code respects these environment variables and will route all requests via AI Gateway.

```tf
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
    arch = "amd64"
    os   = "linux"
    dir  = local.repo_dir
    env = {
        ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/anthropic",
        ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
    }
    ... # other agent configuration
}
```

## External and Desktop Clients

You can also configure AI tools running outside of a Coder workspace, such as local IDE extensions or desktop applications, to connect to AI Gateway. Use the same settings as the in-workspace case, configure the [base URL](#base-urls) and authenticate with a Coder API token.

For base URL setup, the client machine must have network access to the AI Gateway endpoint on your Coder deployment. Clients using [AI Gateway Proxy](../ai-gateway-proxy/index.md) must be able to reach the proxy endpoint and trust its CA certificate.

Users can generate a long-lived API token from the Coder UI or CLI. Follow the instructions at [Sessions and API tokens](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) to create one.

For headless scenarios, first [create a service account](../../../admin/users/headless-auth.md#create-a-service-account), then generate a long-lived token for it.

<details>
<summary>Example</summary>
For clients supporting [base URL](#base-urls), eg. [Claude Code](./claude-code.md):

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/ai-gateway/anthropic"
export ANTHROPIC_AUTH_TOKEN="<your-coder-api-token>"
```

Replace `coder.example.com` with your Coder deployment URL.

For other clients setup [AI Gateway Proxy](../ai-gateway-proxy/index.md). Configure the proxy endpoint and [CA certificates](../ai-gateway-proxy/setup.md#environment-variables):

```sh
export HTTPS_PROXY="https://coder:<your-coder-api-token>@<proxy-host>:8888"
export SSL_CERT_FILE="/path/to/coder-ai-gateway-proxy-ca.pem"
```

For proxy setup details, see [AI Gateway Proxy setup](../ai-gateway-proxy/setup.md).

For BYOK and workspace template examples, see full [Claude Code](./claude-code.md) example.
</details>

For complete setup instructions, see the [supported client examples](#all-supported-clients).

## All Supported Clients

<children></children>

## Learn more

- [AI Gateway Authentication and BYOK](../auth.md)
- [AI Gateway Reference](../reference.md)
