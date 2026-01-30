# Kilo Code

Kilo Code supports both OpenAI and Anthropic providers, allowing full integration with AI Bridge.

## Configuration

<!-- TODO: Add screenshot of Kilo Code settings -->

<div class="tabs">

### Option 1: OpenAI Compatible (Recommended)

1. Open Kilo Code in VS Code.
2. Go to **Settings** / **Configuration**.
3. **Provider**: Select **OpenAI Compatible**.
4. **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/openai/v1`.
5. **API Key**: Enter your **Coder Session Token**.
6. **Model ID**: Enter the model you wish to use (e.g., `gpt-4o`).

### Option 2: Anthropic

1. **Provider**: Select **Anthropic**.
2. **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/anthropic`.
3. **API Key**: Enter your **Coder Session Token**.
4. **Model ID**: Select your desired Claude model.

</div>

### Notes

* The **OpenAI Compatible** provider is recommended for broad compatibility.

---

**References:** [Kilo Code Configuration](https://kilocode.ai/docs/ai-providers/openai-compatible)
