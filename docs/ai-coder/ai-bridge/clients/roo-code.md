# Roo Code

Roo Code is an AI coding assistant that supports both OpenAI and Anthropic providers, making it fully compatible with AI Bridge.

## Configuration

Roo Code allows you to configure providers via the UI.

<!-- TODO: Add screenshot of Roo Code provider settings -->

<div class="tabs">

### Option 1: OpenAI Compatible (Recommended)

1. Open Roo Code in VS Code.
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

* If you encounter issues with the **OpenAI** provider type, use **OpenAI Compatible** to ensure correct endpoint routing.
* Ensure your Coder deployment URL is reachable from your VS Code environment.

---

**References:** [Roo Code Configuration Profiles](https://docs.roocode.com/features/api-configuration-profiles#creating-and-managing-profiles)
