---
title: Client configuration
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../../index.md), which is included with a Premium license.

Once AI Gateway is setup on your deployment, the AI coding tools used by your users will need to be configured to route requests via AI Gateway.

There are two ways to connect AI tools to AI Gateway:

- Base URL configuration (Recommended): Most AI tools allow customizing the base URL for API requests.
  This is the preferred approach when supported.
- AI Gateway Proxy: For tools that don't support base URL configuration, [AI Gateway Proxy](../ai-gateway-proxy/index.md) can intercept traffic and forward it to AI Gateway.

> [!NOTE]
> AI Gateway works with tools running inside or outside of Coder workspaces.
> For non-workspace setup, visit [External and Desktop Clients](#external-and-desktop-clients).

## Base URLs

Most AI coding tools allow the "base URL" to be customized.
In other words, when a request is made to OpenAI's API from your coding tool, the API endpoint such as [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat) will be appended to the configured base.
Therefore, instead of the default base URL of `https://api.openai.com/v1`, you'll need to set it to `https://coder.example.com/api/v2/ai-gateway/openai/v1`.

The exact configuration method varies by client, some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**:
  Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/ai-gateway/openai/v1`
- **Anthropic-compatible clients**:
  Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/ai-gateway/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

If you run a [standalone AI gateway](../standalone.md), point clients at the gateway endpoint and drop the `/api/v2/ai-gateway` path prefix (for example, `https://ai-gateway.example.com/openai/v1` or `https://ai-gateway.example.com/anthropic`).

## Authentication

For information about authenticating with AI Gateway, visit [AI Gateway Authentication](../auth.md).

## Compatibility

The table below shows tested AI clients and their compatibility with AI Gateway.

| Client                            | OpenAI | Anthropic | Bring Your Own Key (BYOK) | Notes                                                                                                                                                  |
|-----------------------------------|--------|-----------|---------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Xum](./xum.md)                   | ✅      | ✅         | -                         |                                                                                                                                                        |
| [Claude Code](./claude-code.md)   | -      | ✅         | ✅                         |                                                                                                                                                        |
| [Codex CLI](./codex.md)           | ✅      | -         | ✅                         |                                                                                                                                                        |
| [OpenCode](./opencode.md)         | ✅      | ✅         | ✅                         |                                                                                                                                                        |
| [Factory](./factory.md)           | ✅      | ✅         | ✅                         |                                                                                                                                                        |
| [Cline](./cline.md)               | ✅      | ✅         | ✅                         |                                                                                                                                                        |
| [Kilo Code](./kilo-code.md)       | ✅      | ✅         | ❌                         |                                                                                                                                                        |
| [VS Code](./vscode.md)            | ✅      | ✅         | ❌                         | VS Code 1.122+ via Custom Endpoint provider. GitHub sign-in not required. Inline suggestions still require GitHub Copilot.                             |
| [JetBrains IDEs](./jetbrains.md)  | ✅      | ✅         | ❌                         | Claude Agent and Codex in JetBrains IDEs 2026.3+.                                                                                                      |
| [Junie](./junie.md)               | ✅      | ✅         | ✅                         | Junie CLI via custom model profiles.                                                                                                                   |
| [Zed](./zed.md)                   | ✅      | ✅         | ❌                         |                                                                                                                                                        |
| [GitHub Copilot](./copilot.md)    | ⚙️     | -         | -                         | Requires [AI Gateway Proxy](../ai-gateway-proxy/index.md). Uses per-user GitHub tokens.                                                                |
| Devin Desktop (formerly Windsurf) | ❌      | ❌         | ❌                         | No option to override base URL.                                                                                                                        |
| Cursor                            | ❌      | ❌         | ❌                         | Override for OpenAI broken ([upstream issue](https://forum.cursor.com/t/requests-are-sent-to-incorrect-endpoint-when-using-base-url-override/144894)). |
| Sourcegraph Amp                   | ❌      | ❌         | ❌                         | No option to override base URL.                                                                                                                        |
| Kiro                              | ❌      | ❌         | ❌                         | No option to override base URL.                                                                                                                        |
| Gemini CLI                        | ❌      | ❌         | ❌                         | No Gemini API support. Upvote [this issue](https://github.com/coder/coder/issues/24804).                                                               |
| Antigravity                       | ❌      | ❌         | ❌                         | No option to override base URL.                                                                                                                        |
|

*Legend: ✅ supported, ⚙️ requires AI Gateway Proxy, ❌ not supported, - not applicable.*

## Configure in-workspace tools

AI coding tools running inside a Coder workspace, such as IDE extensions, can be configured to use AI Gateway.

This section applies when you want template admins to preconfigure tools inside Coder workspaces.
For tools running outside of a workspace, see [External and Desktop Clients](#external-and-desktop-clients).

While users can manually configure these tools with a long-lived API key, template admins can provide a more seamless experience by pre-configuring them.
The template injects a Coder API token and the AI Gateway base URL into the workspace environment.

### Use a scoped token

> [!WARNING]
> `data.coder_workspace_owner.me.session_token` has the owner's full access to the Coder API.
> If you inject it into the workspace, any process there, including an AI agent, can use it to reach the owner's other workspaces, create workspaces, and create more tokens.
> A network sandbox that allows the Coder deployment doesn't prevent this, because AI Gateway is served from the same host as the rest of the Coder API.

Instead, have the template use the session token during the build to create a token with only the `user:read_personal` [scope](../../../admin/users/sessions-tokens.md#api-key-scopes), and inject that token.
AI Gateway accepts this token, and the rest of the Coder API rejects most requests made with it.
The request runs in the provisioner, so the session token doesn't reach the workspace.

This example uses the [`hashicorp/http`](https://registry.terraform.io/providers/hashicorp/http/latest/docs) provider to create the token, and configures Claude Code to route all requests through AI Gateway:

```tf
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    http = {
      source  = "hashicorp/http"
      version = ">= 3.4"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

locals {
  # Create the token only for a started workspace that isn't a prebuild.
  # A prebuild gets its token when a user claims it.
  create_ai_gateway_token = (
    data.coder_workspace.me.start_count > 0 &&
    !data.coder_workspace.me.is_prebuild &&
    data.coder_workspace_owner.me.session_token != ""
  )
  ai_gateway_token = local.create_ai_gateway_token ? jsondecode(data.http.ai_gateway_token[0].response_body).key : ""
}

data "http" "ai_gateway_token" {
  count  = local.create_ai_gateway_token ? 1 : 0
  url    = "${data.coder_workspace.me.access_url}/api/v2/users/me/keys/tokens"
  method = "POST"
  request_headers = {
    "Coder-Session-Token" = data.coder_workspace_owner.me.session_token
    "Content-Type"        = "application/json"
  }
  request_body = jsonencode({
    # timestamp() defers the request to apply, so a plan creates no token.
    token_name = "ai-gateway-${substr(data.coder_workspace.me.id, 0, 8)}-${formatdate("YYYYMMDDhhmmss", timestamp())}"
    scopes     = ["user:read_personal"]
  })

  lifecycle {
    postcondition {
      condition     = self.status_code == 201
      error_message = "Could not create the AI Gateway token: HTTP ${self.status_code}."
    }
  }
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    ANTHROPIC_BASE_URL   = "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/anthropic"
    ANTHROPIC_AUTH_TOKEN = local.ai_gateway_token
  }
  # ... other agent configuration
}
```

The other client pages refer to `local.ai_gateway_token` from this example.

This pattern has these limitations:

- The provisioner must reach the Coder access URL.
- Each workspace start creates a new token.
  Coder doesn't revoke it when the workspace stops, so it stays valid until it expires after the deployment's [default token lifetime](../../../reference/cli/server/index.md#--default-token-lifetime).
  A workspace that runs longer than that loses access to AI Gateway until it restarts.
  To change the lifetime, set `lifetime` in the request body, in nanoseconds.
- AI Gateway attributes requests made with this token to the user, but not to the workspace.
  In a deployment with more than one organization, a request takes the organization of the user's effective group instead of the workspace's organization.
  Refer to [Request organization](../request-organization.md).

The [Claude Code](https://registry.coder.com/modules/coder/claude-code) and [Codex](https://registry.coder.com/modules/coder-labs/codex) registry modules inject the owner's session token when you set `enable_ai_gateway = true`.

## External and Desktop Clients

You can also configure AI tools running outside of a Coder workspace, such as local IDE extensions or desktop applications, to connect to AI Gateway.
Use the same settings as the in-workspace case, configure the [base URL](#base-urls) and authenticate with a Coder API token.

For base URL setup, the client machine must have network access to the gateway endpoint on your Coder deployment.
Clients using [AI Gateway Proxy](../ai-gateway-proxy/index.md) must be able to reach the proxy endpoint and trust its certificate authority (CA) certificate.

Users can generate a long-lived API token from the Coder UI or CLI.
Follow the instructions at [Sessions and API tokens](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) to create one.

For headless scenarios, first [create a service account](../../../admin/users/headless-auth.md#create-a-service-account), then generate a long-lived token for it.

<details>
<summary>Example</summary>
For clients supporting [base URL](#base-urls), for example, [Claude Code](./claude-code.md):

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/ai-gateway/anthropic"
export ANTHROPIC_AUTH_TOKEN="<your-coder-api-token>"
```

Replace `coder.example.com` with your Coder deployment URL.

For other clients setup [AI Gateway Proxy](../ai-gateway-proxy/index.md).
Configure the proxy endpoint and [CA certificates](../ai-gateway-proxy/setup.md#environment-variables):

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
