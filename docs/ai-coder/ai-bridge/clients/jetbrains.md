# JetBrains IDEs

JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.) can connect to AI Bridge using the "Bring Your Own Key" (BYOK) feature in the JetBrains AI Assistant plugin.

## Prerequisites

*   **JetBrains AI Assistant Plugin**: Ensure the plugin is installed and enabled in your IDE.
*   **Coder Session Token**: You will need your Coder session token to authenticate.

## Configuration Steps

1.  **Open Settings**: Go to **Settings** (Windows/Linux) or **Preferences** (macOS) > **Tools** > **AI Assistant** > **Models**.
2.  **Add Provider**: Click the **+** button or select **Bring Your Own API Key** / **Add Provider**.
3.  **Select Provider Type**: Choose **OpenAI** or **Anthropic**, depending on which backend your AI Bridge is proxying to (typically OpenAI-compatible).
4.  **Configure Endpoint**:
    *   **Name**: Give it a recognizable name (e.g., "Coder AI Bridge").
    *   **Endpoint / Base URL**: Enter your AI Bridge URL.
        *   For OpenAI: `https://coder.example.com/api/v2/aibridge/openai/v1`
        *   For Anthropic: `https://coder.example.com/api/v2/aibridge/anthropic`
5.  **Enter API Key**: Paste your **Coder Session Token** in the API Key field.
6.  **Select Models**: The available models should populate automatically. Select the model you wish to use.
7.  **Apply**: Click **Apply** and **OK**.

You can now use the AI Assistant chat and features powered by AI Bridge.
