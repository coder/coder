# Goose

Goose is an AI agent that runs on your machine. It supports custom providers, allowing connection to AI Bridge.

## Configuration

You can configure Goose using the CLI or environment variables.

<div class="tabs">

### Option 1: CLI Configuration

Run the configuration wizard:

```bash
goose configure
```

1.  Select **Configure Providers**.
2.  Select **Add Custom Provider**.
3.  Choose **OpenAI Compatible**.
4.  **Base URL**: `https://coder.example.com/api/v2/aibridge/openai/v1`
5.  **API Key**: Your **Coder Session Token**.

### Option 2: Environment Variables

For a quick setup, you can export the following variables:

```bash
export OPENAI_HOST="https://coder.example.com/api/v2/aibridge/openai/v1"
export OPENAI_API_KEY="<your-coder-session-token>"
```

*Note: Replace `<your-coder-session-token>` with your actual token.*

</div>

---

**References:** [Goose Providers](https://block.github.io/goose/docs/getting-started/providers/)
