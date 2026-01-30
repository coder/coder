# Cline

[Cline](https://cline.bot) is an autonomous coding agent that works with OpenAI and Anthropic providers via AI Bridge.

## Configuration

To configure Cline to use AI Bridge, follow these steps:
![Cline Settings](../../../images/aibridge/clients/cline-setup.png)

<div class="tabs">

### OpenAI Compatible

1. Open Cline in VS Code.
1. Go to **Settings** / **Configuration**.
1. **Provider**: Select **OpenAI Compatible**.
1. **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/openai/v1`.
1. **API Key**: Enter your **[Coder Session Token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.
1. **Model ID** (Optional): Enter the model you wish to use (e.g., `gpt-5.2-codex`).

![Cline OpenAI Settings](../../../images/aibridge/clients/cline-openai.png)

### Anthropic

1. **Provider**: Select **Anthropic**.
1. **API Key**: Enter your **Coder Session Token**.
1. **Base URL**: Enter `https://coder.example.com/api/v2/aibridge/anthropic` after checking **_Use custom base URL_**.
1. **Model ID** (Optional): Select your desired Claude model.

![Cline Anthropic Settings](../../../images/aibridge/clients/cline-anthropic.png)

</div>

**References:** [Cline Configuration](https://github.com/cline/cline)
