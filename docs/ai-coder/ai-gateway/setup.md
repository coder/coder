# Setup

AI Gateway runs inside the Coder control plane (`coderd`), requiring no separate compute to deploy or scale. Once enabled, `coderd` runs the `aibridged` in-memory and brokers traffic to your configured AI providers on behalf of authenticated users.

**Required**:

1. A **Premium** license with the [AI Governance Add-On](../ai-governance.md).
1. Feature must be [enabled](#activation) using the server flag
1. One or more [providers](#configure-providers) API key(s) must be configured

## Activation

You will need to enable AI Gateway explicitly:

```sh
export CODER_AIBRIDGE_ENABLED=true
coder server
# or
coder server --aibridge-enabled=true
```

## Configure Providers

AI Gateway proxies requests to upstream LLM APIs. Configure at least one provider before exposing AI Gateway to end users.

<div class="tabs">

### OpenAI

Set the following when routing [OpenAI-compatible](https://coder.com/docs/reference/cli/server#--aibridge-openai-key) traffic through AI Gateway:

- `CODER_AIBRIDGE_OPENAI_KEY` or `--aibridge-openai-key`
- `CODER_AIBRIDGE_OPENAI_BASE_URL` or `--aibridge-openai-base-url`

The default base URL (`https://api.openai.com/v1/`) works for the native OpenAI service. Point the base URL at your preferred OpenAI-compatible endpoint (for example, a hosted proxy or LiteLLM deployment) when needed.

If you'd like to create an [OpenAI key](https://platform.openai.com/api-keys) with minimal privileges, this is the minimum required set:

![List Models scope should be set to "Read", Model Capabilities set to "Request"](../../images/aibridge/openai_key_scope.png)

### Anthropic

Set the following when routing [Anthropic-compatible](https://coder.com/docs/reference/cli/server#--aibridge-anthropic-key) traffic through AI Gateway:

- `CODER_AIBRIDGE_ANTHROPIC_KEY` or `--aibridge-anthropic-key`
- `CODER_AIBRIDGE_ANTHROPIC_BASE_URL` or `--aibridge-anthropic-base-url`

The default base URL (`https://api.anthropic.com/`) targets Anthropic's public API. Override it for Anthropic-compatible brokers.

Anthropic does not allow [API keys](https://console.anthropic.com/settings/keys) to have restricted permissions at the time of writing (Nov 2025).

### Amazon Bedrock

Set the following when routing [Amazon Bedrock](https://coder.com/docs/reference/cli/server#--aibridge-bedrock-region) traffic through AI Gateway:

- `CODER_AIBRIDGE_BEDROCK_REGION` or `--aibridge-bedrock-region`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY` or `--aibridge-bedrock-access-key`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET` or `--aibridge-bedrock-access-key-secret`
- `CODER_AIBRIDGE_BEDROCK_MODEL` or `--aibridge-bedrock-model`
- `CODER_AIBRIDGE_BEDROCK_SMALL_FAST_MODEL` or `--aibridge-bedrock-small-fast-model`

> [!NOTE]
> `CODER_AIBRIDGE_BEDROCK_BASE_URL` or `--aibridge-bedrock-base-url` may be used instead of `CODER_AIBRIDGE_BEDROCK_REGION`/`--aibridge-bedrock-region`
if you would like to specify a URL which does not follow the form of `https://bedrock-runtime.<region>.amazonaws.com` - for example if using a
proxy between AI Gateway and AWS Bedrock.

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
   - Add a description like "Coder AI Gateway token".
   - Click **Create access key**.
   - Save both the access key ID and secret access key securely.

4. **Configure your Coder deployment** with the credentials:

   ```sh
   export CODER_AIBRIDGE_BEDROCK_REGION=us-east-1
   export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY=<your-access-key-id>
   export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET=<your-secret-access-key>
   coder server
   ```

### GitHub Copilot

GitHub Copilot offers three plans — Individual, Business, and Enterprise —
each with its own API endpoint. Configure one or more `copilot` providers
using the [indexed provider format](#multiple-instances-of-the-same-provider)
depending on which plans your organization uses.
Copilot providers use OAuth app installations for authentication rather than
static API keys.

```sh
# GitHub Copilot (Individual)
export CODER_AIBRIDGE_PROVIDER_0_TYPE=copilot
export CODER_AIBRIDGE_PROVIDER_0_NAME=copilot

# GitHub Copilot Business
export CODER_AIBRIDGE_PROVIDER_1_TYPE=copilot
export CODER_AIBRIDGE_PROVIDER_1_NAME=copilot-business
export CODER_AIBRIDGE_PROVIDER_1_BASE_URL=https://api.business.githubcopilot.com

# GitHub Copilot Enterprise
export CODER_AIBRIDGE_PROVIDER_2_TYPE=copilot
export CODER_AIBRIDGE_PROVIDER_2_NAME=copilot-enterprise
export CODER_AIBRIDGE_PROVIDER_2_BASE_URL=https://api.enterprise.githubcopilot.com
```

The default base URL targets the individual Copilot API
(`api.individual.githubcopilot.com`). Override `CODER_AIBRIDGE_PROVIDER_<N>_BASE_URL`
for Business or Enterprise tiers as shown above.

For client-side setup (proxy, certificates, IDE configuration), see
[GitHub Copilot client configuration](./clients/copilot.md).

### ChatGPT

Configure a ChatGPT provider by creating an `openai`-typed instance with the
ChatGPT Codex base URL:

```sh
export CODER_AIBRIDGE_PROVIDER_0_TYPE=openai
export CODER_AIBRIDGE_PROVIDER_0_NAME=chatgpt
export CODER_AIBRIDGE_PROVIDER_0_BASE_URL=https://chatgpt.com/backend-api/codex
```

</div>

> [!NOTE]
> See the [Supported APIs](./reference.md#supported-apis) section below for precise endpoint coverage and interception behavior.

### Multiple instances of the same provider

You can configure multiple instances of the same provider type — for example, to
route different teams to separate API keys, use different base URLs per region, or
connect to both a direct API and a proxy simultaneously. Use indexed environment
variables following the pattern `CODER_AIBRIDGE_PROVIDER_<N>_<KEY>`:

```sh
# Anthropic routed through a corporate proxy
export CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic
export CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-corp
export CODER_AIBRIDGE_PROVIDER_0_KEY=sk-ant-corp-xxx
export CODER_AIBRIDGE_PROVIDER_0_BASE_URL=https://llm-proxy.internal.example.com/anthropic

# Anthropic direct (for teams that need direct access)
export CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic
export CODER_AIBRIDGE_PROVIDER_1_NAME=anthropic-direct
export CODER_AIBRIDGE_PROVIDER_1_KEY=sk-ant-direct-yyy

# Azure-hosted OpenAI deployment
export CODER_AIBRIDGE_PROVIDER_2_TYPE=openai
export CODER_AIBRIDGE_PROVIDER_2_NAME=azure-openai
export CODER_AIBRIDGE_PROVIDER_2_KEY=azure-key-zzz
export CODER_AIBRIDGE_PROVIDER_2_BASE_URL=https://my-deployment.openai.azure.com/

# Anthropic via AWS Bedrock
export CODER_AIBRIDGE_PROVIDER_3_TYPE=anthropic
export CODER_AIBRIDGE_PROVIDER_3_NAME=anthropic-bedrock
export CODER_AIBRIDGE_PROVIDER_3_BEDROCK_REGION=us-west-2
export CODER_AIBRIDGE_PROVIDER_3_BEDROCK_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
export CODER_AIBRIDGE_PROVIDER_3_BEDROCK_ACCESS_KEY_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

coder server
```

Each provider instance gets a unique route based on its `NAME`. Clients send
requests to `/api/v2/aibridge/<NAME>/` to target a specific instance:

| Instance name       | Route                                               |
|---------------------|-----------------------------------------------------|
| `anthropic-corp`    | `/api/v2/aibridge/anthropic-corp/v1/messages`       |
| `anthropic-direct`  | `/api/v2/aibridge/anthropic-direct/v1/messages`     |
| `azure-openai`      | `/api/v2/aibridge/azure-openai/v1/chat/completions` |
| `anthropic-bedrock` | `/api/v2/aibridge/anthropic-bedrock/v1/messages`    |

**Supported keys per provider:**

| Key        | Required | Description                                          |
|------------|----------|------------------------------------------------------|
| `TYPE`     | Yes      | Provider type: `openai`, `anthropic`, or `copilot`   |
| `NAME`     | No       | Unique instance name for routing. Defaults to `TYPE` |
| `KEY`      | No       | API key for upstream authentication (alias: `KEYS`)  |
| `BASE_URL` | No       | Base URL of the upstream API                         |

For `anthropic` providers using AWS Bedrock, the following keys are also
available: `BEDROCK_BASE_URL`, `BEDROCK_REGION`,
`BEDROCK_ACCESS_KEY` (alias: `BEDROCK_ACCESS_KEYS`),
`BEDROCK_ACCESS_KEY_SECRET` (alias: `BEDROCK_ACCESS_KEY_SECRETS`),
`BEDROCK_MODEL`, `BEDROCK_SMALL_FAST_MODEL`.

> [!NOTE]
> Indices must be contiguous and start at `0`. Each instance must have a unique
> `NAME` — if two instances of the same `TYPE` omit `NAME`, they will both
> default to the type name and fail with a duplicate name error.
>
> The legacy single-provider environment variables (`CODER_AIBRIDGE_OPENAI_KEY`,
> `CODER_AIBRIDGE_ANTHROPIC_KEY`, etc.) continue to work. However, setting both
> a legacy variable and an indexed provider with the same default name (e.g.
> `CODER_AIBRIDGE_OPENAI_KEY` and an indexed provider named `openai`) will
> produce a startup error — remove one or the other to resolve the conflict.

## Data Retention

AI Gateway records prompts, token usage, tool invocations, and model reasoning for auditing and
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

## Structured Logging

AI Gateway can emit structured logs for every interception record, making it
straightforward to export data to external SIEM or observability platforms.

Enable with `--aibridge-structured-logging` or `CODER_AIBRIDGE_STRUCTURED_LOGGING`:

```sh
coder server --aibridge-structured-logging=true
```

Or in YAML:

```yaml
aibridge:
  structured_logging: true
```

These logs are written to the same output stream as all other `coderd` logs,
using the format configured by
[`--log-human`](../../reference/cli/server.md#--log-human) (default, writes to
stderr) or [`--log-json`](../../reference/cli/server.md#--log-json). For machine
ingestion, set `--log-json` to a file path or `/dev/stderr` so that records are
emitted as JSON.

Filter for AI Gateway records in your logging pipeline by matching on the
`"interception log"` message. Each log line includes a `record_type` field that
indicates the kind of event captured:

| `record_type`        | Description                             | Key fields                                                                     |
|----------------------|-----------------------------------------|--------------------------------------------------------------------------------|
| `interception_start` | A new intercepted request begins.       | `interception_id`, `initiator_id`, `provider`, `model`, `client`, `started_at` |
| `interception_end`   | An intercepted request completes.       | `interception_id`, `ended_at`                                                  |
| `token_usage`        | Token consumption for a response.       | `interception_id`, `input_tokens`, `output_tokens`, `created_at`               |
| `prompt_usage`       | The last user prompt in a request.      | `interception_id`, `prompt`, `created_at`                                      |
| `tool_usage`         | A tool/function call made by the model. | `interception_id`, `tool`, `input`, `server_url`, `injected`, `created_at`     |
| `model_thought`      | Model reasoning or thinking content.    | `interception_id`, `content`, `created_at`                                     |
