# Setup

AI Bridge runs inside the Coder control plane (`coderd`), requiring no separate compute to deploy or scale. Once enabled, `coderd` runs the `aibridged` in-memory and brokers traffic to your configured AI providers on behalf of authenticated users.

**Required**:

1. A **Premium** license with the [AI Governance Add-On](../ai-governance.md).
1. Feature must be [enabled](#activation) using the server flag
1. One or more [providers](#configure-providers) API key(s) must be configured

## Prerequisites

Before enabling AI Bridge:

- Ensure your Coder deployment is running **v2.30 or later**.
- If your environment uses a **corporate HTTP proxy**, confirm the proxy path to
  your model provider endpoints (e.g., `api.openai.com`,
  `api.anthropic.com`) is open and pre-approved by your network security team.
  See [AI Bridge Proxy](./ai-bridge-proxy.md) for proxy configuration details.
- Have API keys ready for the LLM providers you want to use.

## Activation

Enable AI Bridge explicitly:

```sh
CODER_AIBRIDGE_ENABLED=true coder server
# or
coder server --aibridge-enabled=true
```

## Configure Providers

AI Bridge proxies requests to upstream LLM APIs. Configure at least one provider before exposing AI Bridge to end users.

<div class="tabs">

### OpenAI

Set the following when routing [OpenAI-compatible](https://coder.com/docs/reference/cli/server#--aibridge-openai-key) traffic through AI Bridge:

- `CODER_AIBRIDGE_OPENAI_KEY` or `--aibridge-openai-key`
- `CODER_AIBRIDGE_OPENAI_BASE_URL` or `--aibridge-openai-base-url`

The default base URL (`https://api.openai.com/v1/`) works for the native OpenAI service. Point the base URL at your preferred OpenAI-compatible endpoint (for example, a hosted proxy or LiteLLM deployment) when needed.

If you'd like to create an [OpenAI key](https://platform.openai.com/api-keys) with minimal privileges, this is the minimum required set:

![List Models scope should be set to "Read", Model Capabilities set to "Request"](../../images/aibridge/openai_key_scope.png)

### Anthropic

Set the following when routing [Anthropic-compatible](https://coder.com/docs/reference/cli/server#--aibridge-anthropic-key) traffic through AI Bridge:

- `CODER_AIBRIDGE_ANTHROPIC_KEY` or `--aibridge-anthropic-key`
- `CODER_AIBRIDGE_ANTHROPIC_BASE_URL` or `--aibridge-anthropic-base-url`

The default base URL (`https://api.anthropic.com/`) targets Anthropic's public API. Override it for Anthropic-compatible brokers.

Anthropic does not allow [API keys](https://console.anthropic.com/settings/keys) to have restricted permissions at the time of writing (Nov 2025).

### Amazon Bedrock

Set the following when routing [Amazon Bedrock](https://coder.com/docs/reference/cli/server#--aibridge-bedrock-region) traffic through AI Bridge:

- `CODER_AIBRIDGE_BEDROCK_REGION` or `--aibridge-bedrock-region`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY` or `--aibridge-bedrock-access-key`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET` or `--aibridge-bedrock-access-key-secret`
- `CODER_AIBRIDGE_BEDROCK_MODEL` or `--aibridge-bedrock-model`
- `CODER_AIBRIDGE_BEDROCK_SMALL_FAST_MODEL` or `--aibridge-bedrock-small-fast-model`

> [!NOTE]
> `CODER_AIBRIDGE_BEDROCK_BASE_URL` or `--aibridge-bedrock-base-url` may be used instead of `CODER_AIBRIDGE_BEDROCK_REGION`/`--aibridge-bedrock-region`
if you would like to specify a URL which does not follow the form of `https://bedrock-runtime.<region>.amazonaws.com` - for example if using a
proxy between AI Bridge and AWS Bedrock.

#### Obtaining Bedrock credentials

1. **Choose a region** where you want to use Bedrock.

2. **Generate API keys** in the [AWS Bedrock console](https://us-east-1.console.aws.amazon.com/bedrock/home?region=us-east-1#/api-keys/long-term/create) (replace `us-east-1` in the URL with your chosen region):
   - Choose an expiry period for the key.
   - Click **Generate**.
   - This creates an IAM user with strictly-scoped permissions for Bedrock access.

3. **Create an access key** for the IAM user:
   - After generating the API key, click **"You can directly modify permissions for the IAM user associated"**.
   - In the IAM user page, navigate to the **Security credentials** tab.
   - Under **Access keys**, click **Create access key**.
   - Select **"Application running outside AWS"** as the use case.
   - Click **Next**.
   - Add a description like "Coder AI Bridge token".
   - Click **Create access key**.
   - Save both the access key ID and secret access key securely.

4. **Configure your Coder deployment** with the credentials:

   ```sh
   export CODER_AIBRIDGE_BEDROCK_REGION=us-east-1
   export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY=<your-access-key-id>
   export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET=<your-secret-access-key>
   coder server
   ```

### Additional providers and model proxies

If you use an internal LLM gateway (such as LiteLLM, Portkey, or a custom
proxy), you can point AI Bridge at your gateway as the upstream endpoint instead
of pointing directly at the provider. This lets Bridge handle authentication
and audit logging while your existing gateway handles routing, load balancing,
and failover.

Set the base URL for the upstream provider:

```bash
CODER_AIBRIDGE_OPENAI_BASE_URL=https://your-internal-gateway.example.com/v1
```

Bridge is complementary to existing gateways — it adds Coder-level identity
attribution and audit logging on top of your existing routing infrastructure.

> **Note:** See the
> [Supported APIs](/docs/ai-coder/ai-bridge/reference#supported-apis) section
> for precise endpoint coverage and interception behavior.

## Configure templates

Once AI Bridge is enabled at the server level, configure your workspace
templates to inject the Bridge base URLs and session tokens. This ensures agents
inside workspaces route through Bridge automatically — developers see zero
additional setup.

```hcl
data "coder_workspace_owner" "me" {}
data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir

  env = {
    # Route Anthropic traffic through AI Bridge
    ANTHROPIC_BASE_URL = "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic"
    ANTHROPIC_API_KEY  = data.coder_workspace_owner.me.session_token

    # Route OpenAI traffic through AI Bridge
    OPENAI_BASE_URL    = "${data.coder_workspace.me.access_url}/api/v2/aibridge/openai/v1"
    OPENAI_API_KEY     = data.coder_workspace_owner.me.session_token
  }
}
```

With this template configuration:

- Developers launch their workspace and AI tools just work.
- No API key management or provisioning tickets.
- The Coder session token replaces the provider API key — developers never hold
  provider credentials.
- All AI traffic is automatically routed through Bridge for audit and
  attribution.

For per-client configuration details, see
[Client Configuration](./clients/index.md).

## Data Retention

AI Bridge records prompts, token usage, and tool invocations for auditing and
monitoring purposes. By default, this data is retained for **60 days**.

Configure retention using `--aibridge-retention` or `CODER_AIBRIDGE_RETENTION`:

```sh
coder server --aibridge-retention=90d
```

Or in YAML:

```yaml
aibridge:
  retention: 90d
```

Set to `0` to retain data indefinitely.

For duration formats, how retention works, and best practices, see the
[Data Retention](../../admin/setup/data-retention.md) documentation.

## Verify the setup

After enabling AI Bridge and configuring a template:

1. Launch a workspace from the configured template.
2. Run an AI tool (e.g., Claude Code) inside the workspace.
3. Verify that the request appears in the Coder audit logs or via the
   [REST API / CLI export](./monitoring.md#exporting-data).
4. Check that the response is returned successfully through Bridge.

If you have the [Grafana dashboard](./monitoring.md) imported, you should see
the request appear in the AI Bridge dashboards.

## Next steps

- [Client Configuration](./clients/index.md) — Configure specific AI tools to
  use Bridge.
- [Monitoring](./monitoring.md) — Set up observability, export data, and
  configure tracing.
- [MCP Tools Injection](./mcp.md) — Centrally configure MCP servers.
- [AI Bridge Proxy](./ai-bridge-proxy.md) — Support tools behind corporate
  proxies or without base URL overrides.
- [Reference](./reference.md) — Full configuration reference.
