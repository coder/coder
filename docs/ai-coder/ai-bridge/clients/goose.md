# Goose

Goose is an AI agent that runs on your machine. It supports custom providers, allowing connection to AI Bridge.

## Configuration

You can configure Goose using the CLI or environment variables.

<div class="tabs">

### OpenAI Compatible

<div class="tabs">

#### CLI Configuration

Run the configuration wizard:

```bash
goose configure
```

1. Select **Configure Providers**.
2. Select **Add Custom Provider**.
3. Choose **OpenAI Compatible**.
4. **Base URL**: `https://coder.example.com/api/v2/aibridge/openai/v1`
5. **API Key**: Your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

#### Environment Variables

For a quick setup, you can export the following variables:

```bash
export OPENAI_HOST="https://coder.example.com/api/v2/aibridge/openai/v1"
export OPENAI_API_KEY="<your-coder-session-token>"
```

### Anthropic

<div class="tabs">

#### CLI Configuration

Run the configuration wizard:

```bash
goose configure
```

1. Select **Configure Providers**.
2. Select **Add Custom Provider**.
3. Choose **Anthropic**.
4. **Base URL**: `https://coder.example.com/api/v2/aibridge/anthropic/v1`
5. **API Key**: Your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

#### Environment Variables

For a quick setup, you can export the following variables:

```bash
export ANTHROPIC_HOST="https://coder.example.com/api/v2/aibridge/anthropic/v1"
export ANTHROPIC_API_KEY="<your-coder-session-token>"
```

</div>

</div>

Goose will now route requests through AI Bridge.

Goose Desktop shares the same configuration as the CLI. You can set environment variables in your system or use the configuration wizard in the app.

**References:** [Goose Providers](https://block.github.io/goose/docs/getting-started/providers/)
