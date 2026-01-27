# Cline

Cline is an autonomous coding agent that works with OpenAI and Anthropic providers via AI Bridge.

## Configuration

<!-- TODO: Add screenshot of Cline configuration interface -->

<div class="tabs">

### Option 1: OpenAI Compatible (Recommended)

1.  Open Cline in VS Code.
2.  Go to **Settings** / **Configuration**.
3.  **Provider**: Select **OpenAI Compatible**.
4.  **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/openai/v1`.
5.  **API Key**: Enter your **Coder Session Token**.
6.  **Model ID**: Enter the model you wish to use (e.g., `gpt-4o`).

### Option 2: Anthropic

1.  **Provider**: Select **Anthropic**.
2.  **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/anthropic`.
3.  **API Key**: Enter your **Coder Session Token**.
4.  **Model ID**: Select your desired Claude model.

</div>

### Notes

*   If using the **OpenAI** provider type, you must override the Base URL in the advanced settings. The **OpenAI Compatible** provider is generally easier to configure.

---

**References:** [Cline Configuration](https://github.com/cline/cline)
