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

Virtual desktop requires the [portabledesktop](https://registry.coder.com/modules/coder/portabledesktop) module installed in the workspace template.

### Choose the computer use model

Computer use runs on a chat model from the chat's organization.
Each organization picks its own model as an organization-scoped [model override](../models.md#model-overrides).
To change it, you need permission to edit that organization's models.

1. Go to **Admin settings** > **AI** > **Coder Agents**.
1. If the **Organization settings** section shows an organization picker, select the organization.
1. In the **Computer use** row, select a model.
1. Select **Save**.

The **Computer use** row appears only while the `chat-virtual-desktop` experiment is enabled.
The list shows the organization's enabled models whose AI provider type is Anthropic or OpenAI, because computer use supports only those provider types.
To add a model to the list, enable an Anthropic or OpenAI model on the **Models** page.

When no model is selected, the row shows **Use default model** and computer use runs on the organization's default model.
To return to the default model, select **Clear**, then select **Save**.
If the selected model is later disabled, deleted, or unusable for the chat owner, computer use falls back to the organization's default model.

Computer use is unavailable in a chat when the chat owner has no usable [credentials](../models.md#credential-selection) for the AI provider of the model it runs on.
It's also unavailable when it runs on the organization's default model and that model isn't an Anthropic or OpenAI model.
OpenAI computer use requires the Responses API, so it fails on an OpenAI model whose **Use Responses API** override is set to Off.

A warning icon next to the model selector means that Coder hasn't verified the model for computer use.
The icon follows the model that computer use runs on: the selected model, or the organization's default model when no model is selected.
The warning doesn't block saving.
Before you save, confirm that the model supports the provider's computer use tool.

The same setting is available through the API at `PUT /api/v2/organizations/{organization}/chats/model-overrides/computer_use`.
The request fails with `404 Not Found` while the experiment is disabled, and with `400 Bad Request` when the model's AI provider type is not Anthropic or OpenAI.
