# Setup

Bridge runs inside the Coder control plane, requiring no separate compute to deploy or scale. Once enabled, `coderd` hosts the bridge in-memory and brokers traffic to your configured AI providers on behalf of authenticated users.

**Required**:

1. A **premium** licensed Coder deployment
1. Feature must be [enabled](#activation) using the server flag
1. One or more [provider](#configure-providers) API key(s) must be configured

### Activation

You will need to enable AI Bridge explicitly:

```sh
CODER_AIBRIDGE_ENABLED=true coder server
# or
coder server --aibridge-enabled=true
```

### Configure providers

Bridge proxies requests to upstream LLM APIs. Configure at least one provider before exposing Bridge to end users.

#### OpenAI

Set the following when routing OpenAI-compatible traffic through Bridge:

- `CODER_AIBRIDGE_OPENAI_KEY` or `--aibridge-openai-key`
- `CODER_AIBRIDGE_OPENAI_BASE_URL` or `--aibridge-openai-base-url`

The default base URL (`https://api.openai.com/v1/`) works for the native OpenAI service. Point the base URL at your preferred OpenAI-compatible endpoint (for example, a hosted proxy or LiteLLM deployment) when needed.

#### Anthropic

Set the following when routing Anthropic-compatible traffic through Bridge:

- `CODER_AIBRIDGE_ANTHROPIC_KEY` or `--aibridge-anthropic-key`
- `CODER_AIBRIDGE_ANTHROPIC_BASE_URL` or `--aibridge-anthropic-base-url`

The default base URL (`https://api.anthropic.com/`) targets Anthropic's public API. Override it for Anthropic-compatible brokers.

##### Amazon Bedrock

Set the following when routing Amazon Bedrock traffic through Bridge:

- `CODER_AIBRIDGE_BEDROCK_REGION` or `--aibridge-bedrock-region`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY` or `--aibridge-bedrock-access-key`
- `CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET` or `--aibridge-bedrock-access-key-secret`
- `CODER_AIBRIDGE_BEDROCK_MODEL` or `--aibridge-bedrock-model`
- `CODER_AIBRIDGE_BEDROCK_SMALL_FAST_MODEL` or `--aibridge-bedrock-small-fast-model`

#### Additional providers and Model Proxies

Bridge can relay traffic to other OpenAI- or Anthropic-compatible services or model proxies like LiteLLM by pointing the base URL variables above at the provider you operate. Share feedback or follow along in the [`aibridge`](https://github.com/coder/aibridge) issue tracker as we expand support for additional providers.

> [!NOTE]
> See the [Supported APIs](../reference#supported-apis) section below for precise endpoint coverage and interception behavior.
