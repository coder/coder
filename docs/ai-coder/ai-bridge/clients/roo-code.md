# Roo Code

Roo Code is an AI coding assistant that supports OpenAI-compatible providers, making it compatible with AI Bridge.

## Configuration

1.  Open Roo Code in VS Code.
2.  Go to **Settings** / **Configuration**.
3.  **Provider**: Select **OpenAI Compatible**.
4.  **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/openai/v1`.
5.  **API Key**: Enter your **Coder Session Token**.
6.  **Model ID**: Enter the model you wish to use (e.g., `gpt-4o`).

### Notes

*   Ensure you use the legacy format/provider if prompted, to avoid issues with `/v1/responses` endpoints if not yet supported by your specific setup.
*   AI Bridge supports both OpenAI and Anthropic endpoints, so you may also be able to configure it as an Anthropic provider with the base URL `https://coder.example.com/api/v2/aibridge/anthropic`.
