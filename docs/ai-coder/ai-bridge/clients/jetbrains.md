# JetBrains IDEs

JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.) support AI Bridge via the "Bring Your Own Key" (BYOK) feature.

## Prerequisites

* **JetBrains AI Assistant Plugin**: Installed and enabled.
* **Authentication**: Your Coder Session Token.

## Configuration Steps

<!-- TODO: Add screenshot of JetBrains AI Assistant "Add Provider" dialog -->

1. **Open Settings**: Go to **Settings** (Windows/Linux) or **Preferences** (macOS) > **Tools** > **AI Assistant** > **Models**.
2. **Add Provider**: Click the **+** button or select **Bring Your Own API Key** / **Add Provider**.
3. **Select Provider Type**:
    * **OpenAI**: If you are routing to an OpenAI model via AI Bridge.
    * **Anthropic**: If you are routing to a Claude model via AI Bridge.
4. **Configure Endpoint**:
    * **Name**: Enter a recognizable name (e.g., "Coder - OpenAI" or "Coder - Anthropic").
    * **Endpoint**: Enter the corresponding AI Bridge URL:
        * **OpenAI**: `https://coder.example.com/api/v2/aibridge/openai/v1`
        * **Anthropic**: `https://coder.example.com/api/v2/aibridge/anthropic`
5. **Enter API Key**: Paste your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.
6. **Select Models**: The available models should populate automatically.
7. **Apply**: Click **Apply** and **OK**.

You can now use the AI Assistant chat with the configured provider.

**References:** [Use custom models with JetBrains AI Assistant](https://www.jetbrains.com/help/ai-assistant/use-custom-models.html)
