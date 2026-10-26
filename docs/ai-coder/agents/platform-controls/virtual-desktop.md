---
title: Virtual desktop
---

> [!NOTE]
> This feature is experimental.
> Pin a release before broad rollout and review the release notes before upgrading.

## Enable the experiment

```sh
coder server --experiments=chat-virtual-desktop
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=chat-virtual-desktop
```

## What it does

Lets agents drive a graphical desktop inside the workspace through `spawn_agent` with `type=computer_use` (screenshots, mouse, keyboard).

## Configuration

Once the experiment is enabled, configure the computer-use provider under **Admin settings** > **AI** > **Coder Agents** > **Deployment settings**.
Use the **Virtual desktop** card.

Choose a **Computer use provider**.
The list shows every enabled AI provider of type Anthropic or OpenAI, labeled with its type.
Computer-use turns and the `computer_use` API key check use the selected provider.
If no Anthropic or OpenAI AI provider is enabled, add one under the **Providers** sidebar item first.

Virtual desktop also requires:

- The [portabledesktop](https://registry.coder.com/modules/coder/portabledesktop) module installed in the workspace template.
- An API key for the selected AI provider.

The Anthropic and OpenAI computer-use models are fixed by Coder per provider type and are not selectable from this UI.

When no AI provider is selected, computer use uses the first enabled AI provider of the configured type, sorted by name.
Anthropic is the default type when no type is set.
If the selected AI provider is later disabled or deleted, `computer_use` subagents are unavailable until you select another provider.

The same configuration is available at:

- `GET /api/v2/chats/config/computer-use-provider`
- `PUT /api/v2/chats/config/computer-use-provider`
