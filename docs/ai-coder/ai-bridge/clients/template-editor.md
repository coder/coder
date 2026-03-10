# Template Editor AI Assistant

The template editor includes a built-in AI coding assistant that can read, edit,
and delete files directly within the browser-based template version editor. It
uses [AI Bridge](../index.md) to route requests to your configured model
provider.

> [!NOTE]
> This feature requires a [Premium license](https://coder.com/pricing).

## Prerequisites

- [AI Bridge](../setup.md) is enabled and configured with at least one provider.
- The `ai-template-editor` [experiment](../../../install/releases/feature-stages.md#early-access-features)
  is enabled on your Coder deployment.

## How to use

1. Navigate to any template and click **Edit** to open the template version
   editor.
1. Click the **sparkle (✨) button** in the editor toolbar to open the AI
   assistant panel.
1. Type a prompt describing the changes you want (for example, _"Add a
   Docker sidecar for Postgres"_).
1. The assistant proposes file edits that require your explicit approval before
   they are applied. You can approve or reject each change individually.
1. After approving edits you can ask the assistant to **build** the template to
   validate the Terraform configuration, and then **publish** a new version —
   all from within the chat.

## Capabilities

| Action                  | Description                                                                                              |
|-------------------------|----------------------------------------------------------------------------------------------------------|
| **Read files**          | The assistant can read any file in the template's file tree.                                             |
| **Edit / create files** | Proposes diffs that you approve before they are applied.                                                 |
| **Delete files**        | Proposes file deletions that you approve before they are applied.                                        |
| **Build template**      | Triggers a dry-run provisioner build to validate Terraform.                                              |
| **Publish version**     | Promotes the current editor state to a new template version.                                             |
| **Registry lookup**     | Queries the [Coder Registry](https://registry.coder.com) for modules and examples via MCP (best-effort). |

## Model selection

The assistant automatically selects a chat-capable model from your AI Bridge
configuration. If multiple models are available, you can switch between them
using the model selector at the top of the chat panel.

## Limitations

- Chat history is **ephemeral** — it is not persisted across page reloads or
  browser sessions.
- The assistant operates only on the files visible in the template version
  editor. It cannot access files outside the template or run arbitrary commands
  in a workspace.
- Registry tool availability depends on network connectivity to
  `registry.coder.com`. If the connection fails, the assistant continues to
  work with local file operations and displays a warning banner.
