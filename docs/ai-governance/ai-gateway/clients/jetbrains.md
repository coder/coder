---
title: JetBrains
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../../index.md), which is included with a Premium license.

JetBrains IDE (IntelliJ IDEA, PyCharm, WebStorm, and so on) support AI Gateway via the [third-party model configuration](https://www.jetbrains.com/help/ai-assistant/use-custom-models.html#provide-your-own-api-key) feature.
In JetBrains IDEs 2026.3 and later, Claude Agent and Codex in JetBrains Air can also use AI Gateway from a Coder workspace.
For details, refer to [Coding agents in JetBrains Air](#coding-agents-in-jetbrains-air).

## Prerequisites

* [**JetBrains AI Assistant**](https://www.jetbrains.com/help/ai-assistant/installation-guide-ai-assistant.html): Installed and enabled.
* **Authentication**:
  Your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.

## Centralized API Key

1. **Open Settings**:
   Go to **Settings** > **Tools** > **AI Assistant** > **Models & API Keys**.
1. **Configure Provider**:
   Go to **Third-party AI providers**.
1. **Choose Provider**:
   Choose **OpenAI-compatible**.
1. **URL**:
   `https://coder.example.com/api/v2/ai-gateway/openai/v1`
1. **API Key**:
   Paste your **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.
1. **Apply**:
   Select **Apply** and **OK**.

![JetBrains AI Assistant Settings](../../../images/aibridge/clients/jetbrains-ai-settings.png)

<a id="using-the-ai-assistant"></a>

## Use the AI Assistant

1. Go back to **AI Chat** on theleft side bar and choose **Chat**.
1. In the Model dropdown, select the desired model (for example, `gpt-5.2`).

![JetBrains AI Assistant Chat](../../../images/aibridge/clients/jetbrains-ai-chat.png)

You can now use the AI Assistant chat with the configured provider.

> [!NOTE]
>
> * JetBrains AI Assistant currently only supports OpenAI-compatible endpoints.
>   There is an open [issue](https://youtrack.jetbrains.com/issue/LLM-22740) tracking support for Anthropic.
> * JetBrains AI Assistant might not support all models that support OpenAI's `/chat/completions` endpoint in Chat mode.
> * JetBrains AI Assistant does not send an identifying user agent, so AI Gateway records these sessions with the client `Unknown`.
>   Usage, tokens, and audit data are still captured.

## BYOK (Personal API Key)

> [!NOTE]
> At the time of writing, JetBrains AI Assistant does not support sending custom headers, so Bring Your Own Key (BYOK) mode is not available.

## Coding agents in JetBrains Air

JetBrains IDEs 2026.3 and later include [JetBrains Air](https://www.jetbrains.com/help/idea/agents-in-ide.html), an agent workspace provided by the bundled Air plugin.
When you connect to a Coder workspace, Air runs agents inside the workspace and [detects the agents installed there](https://www.jetbrains.com/help/air-ides/air-configure-authorization.html), along with their existing configuration.
If the workspace is configured for AI Gateway, Claude Agent and Codex sessions route requests through AI Gateway without any configuration in the IDE.
You don't need a JetBrains AI subscription.

| Agent        | How Air runs it                                    | Settings read from the workspace                        |
|--------------|----------------------------------------------------|---------------------------------------------------------|
| Claude Agent | Installs its own copy from the ACP agent registry  | `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`         |
| Codex        | Runs the Codex CLI installed in the workspace      | The AI Gateway model provider in `~/.codex/config.toml` |

To configure this for every workspace, set `enable_ai_gateway = true` on the [Claude Code module](./claude-code.md#pre-configuring-in-templates) and the [Codex module](./codex.md#pre-configuring-in-templates).
For Claude Agent, you can instead set the environment variables on the workspace agent, as shown in [Configure in-workspace tools](./index.md#configure-in-workspace-tools).

To start a session:

1. Connect to the workspace with [JetBrains Toolbox](../../../workspaces/workspace-access/jetbrains/toolbox.md) or [JetBrains Gateway](../../../workspaces/workspace-access/jetbrains/gateway.md).
1. Open the **Agent Sessions** tool window.
1. Next to **New Session**, select a Claude Agent or Codex preset.
1. Send a prompt.

The session appears in the AI Gateway [sessions list](../audit.md#sessions-list).

> [!NOTE]
> Keep the session on the **Agent login** authorization source, which Claude Agent lists as **Claude subscription** with an **OAuth token**.
> This label is expected, because `ANTHROPIC_AUTH_TOKEN` holds your Coder token.
> If you select **JetBrains AI** or a configured provider instead, requests go to that provider and bypass AI Gateway.

**References:**
[Use custom models with JetBrains AI Assistant](https://www.jetbrains.com/help/ai-assistant/use-custom-models.html#provide-your-own-api-key),
[Agents in IntelliJ IDEA](https://www.jetbrains.com/help/idea/agents-in-ide.html)
