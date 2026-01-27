# Copilot

GitHub Copilot (VS Code Extension) supports connecting to OpenAI-compatible endpoints via the "Bring your own model" feature in its pre-release versions.

## Configuration

### Prerequisites

*   **VS Code Extension**: You typically need the **Pre-release** version of the GitHub Copilot Chat extension to access these features.
*   **Anthropic Support**: ❌ Not supported (only OpenAI-compatible).

### Steps

1.  Open your VS Code `settings.json`.
2.  Add the configuration for `github.copilot.advanced`.

```json
"github.copilot.advanced": {
    "debug.testOverrideProxyUrl": "https://coder.example.com/api/v2/aibridge/openai/v1",
    "debug.overrideProxyUrl": "https://coder.example.com/api/v2/aibridge/openai/v1"
}
```

*Note: The setting names may change as the feature moves from pre-release to stable. Refer to the official documentation for the latest setting keys.*

### Authentication

Set the `OPENAI_API_KEY` environment variable to your **Coder Session Token** before launching VS Code, or configure it within the extension settings if a specific API Key field is available for the custom endpoint.

---

**References:** [GitHub Copilot - Bring your own language model](https://code.visualstudio.com/docs/copilot/customization/language-models#_add-an-openaicompatible-model)
