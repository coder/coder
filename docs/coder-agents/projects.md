---
title: Projects
---

> [!NOTE]
> Projects are in [early access](../reference/feature-stages.md#early-access-features).
> Behavior and APIs might change between releases.

Projects group related Coder Agents chats.
Every chat in a project gets the project's instructions, skills, and memory, so you don't repeat the same context in each chat.
The project also sets which templates its chats can use for workspaces.
Your projects appear in the **Projects** section of the **Agents** page sidebar.

## What a project provides

| Capability                    | What it does                                                             |
|-------------------------------|--------------------------------------------------------------------------|
| [Instructions](#instructions) | Text added to the system prompt of every chat in the project.            |
| [Skills](#skills)             | Skills available only to chats in the project.                           |
| [Memory](#memory)             | Durable notes that the agent saves in one chat and reads in later chats. |
| [Templates](#templates)       | The templates that chats in the project can use for workspaces.          |

## Ownership

Each project belongs to one organization and is owned by the user who created it.
You choose the organization when you create the project, and you can't change it later.
Chats in a project must be in the project's organization.

Any organization member who can use Coder Agents can create projects in that organization.
Each user can own up to 100 projects across all organizations.

A project is private to its owner.
Coder records project, instruction, skill, and memory changes in the [audit log](../admin/security/audit-logs.md).

## Create a project

1. On the **Agents** page, select **New project** in the **Projects** section of the sidebar.
1. Enter a **Project name** and an optional **Description**, and pick an icon.
1. Choose the **Organization**.
1. Under **Templates**, select the templates that chats in the project can use.

   You can skip this step and add templates later.
   Until the project has templates, its chats can't create or attach workspaces.
   To also set a default template, refer to [Default template](#default-template).

1. Select **Create project**.

The project page shows a **Details** panel with the project owner, creation date, organization, template count, skills, and instructions, and lists your chats in the project.

To rename or delete a project, open the project's menu on its page or in the sidebar and select **Edit project** or **Delete project**.
Deleting a project deletes its instructions, skills, and memories.
Its chats are kept and move back to your chat list.

## Add chats to a project

- Start a chat from the project page.
  The composer is locked to the project's organization.
- In the general new-chat composer, choose the project in the project picker.
  The picker defaults to **No project**.
- Move an existing chat: open the chat's menu, select **Move to project**, and choose a project.
  Select **Remove from project** to take it out.
- Drag a chat in the sidebar onto a project folder.

You can move only top-level chats that you own, that aren't archived, and that aren't running.
Sub-agent chats always follow their top-level chat.
After a move, the chat's next turn uses the new project's instructions, skills, and memory.
The chat can use only the new project's templates, but it keeps any workspace it already has.

The API equivalent is `PATCH /api/v2/chats/{chat}` with `project_id`.
Set `project_id` to the nil universally unique identifier (UUID) (`00000000-0000-0000-0000-000000000000`) to remove a chat from its project.

## Instructions

Project instructions are added to the system prompt of every chat in the project, including sub-agent chats.
Use them for conventions, context, and preferences that apply to all work in the project.

To add or change them, select **Create** or the edit icon under **Instructions** in the project's **Details** panel.
Chats read instructions on every turn, so a change takes effect on each chat's next turn.

Instructions use the deployment's prompt size limit, set with `--chat-max-prompt-bytes` (default 128&nbsp;KiB).

## Skills

Project skills are single-file `SKILL.md` skills that only chats in the project can load.
Manage them by selecting the **Skills** row in the project's **Details** panel.

A project skill can show as a one-click action above the project's chat composers, or be limited to manual use from the `/` menu.
For details on the format, limits, and how skills from different sources resolve, visit [Project skills](./extending-agents.md#project-skills).

## Memory

Each project has a set of memories: short, named notes that persist across every chat in the project.
The agent manages them with four tools:

| Tool                 | Purpose                                                                        |
|----------------------|--------------------------------------------------------------------------------|
| `read_memory`        | Read a memory's full body.                                                     |
| `save_memory`        | Save a new memory.                                                             |
| `delete_memory`      | Delete a memory.                                                               |
| `consolidate_memory` | Delete and save several memories in one atomic step, to merge or replace them. |

The agent saves facts that matter in future chats without being asked, such as how people on the project work, corrections, decisions, deadlines, and where to find information outside the codebase.
It doesn't save what it can derive from the code or Git history.
The agent is told not to save secrets, credentials, or personal details in memory.
You can also ask the agent to remember or forget something.

How memory reaches the agent:

- Each chat sees an index of memory names and one-line descriptions, and reads a full memory only when it needs it.
- A change made in one chat reaches other chats at the start of their next turn.
- Only top-level chats get the memory tools.
  Sub-agent chats don't read or write memory.
- Memories can't be edited.
  To change one, the agent replaces it.
- When a chat leaves a project, its next turn drops the memories it saw.

| Limit                | Value                                                                     |
|----------------------|---------------------------------------------------------------------------|
| Memories per project | 200. The agent is prompted to consolidate as the project nears the limit. |
| Name                 | Lowercase letters, digits, `-`, and `_`, up to 64 characters.             |
| Description          | 150 characters.                                                           |
| Body                 | 8&nbsp;KB.                                                                |

The dashboard doesn't show memories yet.
To list, read, create, or delete them directly, use `/api/experimental/organizations/{organization}/chats/projects/{project}/memories`.

## Templates

A project sets which of its organization's templates its chats can use.
The agent in a project chat can only create workspaces from those templates.
To give a project chat an existing workspace, the workspace must be built from one of the project's templates.
This applies when you pick a workspace as you start the chat, where the composer lists only those workspaces, and when you set `workspace_id` through the API.
The API rejects other workspaces with `This workspace's template is not allowed in the chat's project.`
A chat that already has a workspace keeps it, as [Template changes and existing workspaces](#template-changes-and-existing-workspaces) describes.
When a project has no templates, the agent in its chats can't create a workspace, and you can't give its chats an existing workspace.
If the agent in a project chat says that no templates are available, add templates to the project.

To change a project's templates, select the template count, such as **2 templates**, in the **Templates** row of the project's **Details** panel.
This opens the **Edit templates** dialog.
You need the `chat_project:update` permission on the project, which the project owner has.
The dialog checks for template changes made elsewhere, such as in another browser tab, when it opens, when you return to its browser tab, and again just before it saves.
If you haven't edited anything, the dialog switches to the newer templates.
If you have unsaved edits, it shows a warning and you can't save until you choose.
Select **Load latest** to start over from the newer templates, or **Keep my draft** to replace them with yours when you save.
If the check fails, saving is paused until you select **Retry**.

You can add any template in the project's organization that you can access, that allows Coder Agents, and that isn't deprecated.
**Select all** adds every such template.
If a template in the project's list is deleted, deprecated, or no longer allows agents, the dialog warns that saving removes it.
When that template is the default template, saving also removes the default template and its default values.
The project's list never overrides the administrator setting: the agent can't use a template that doesn't allow Coder Agents, even when it's in the list.
For that setting, refer to [Template routing](./platform-controls/index.md#template-routing).

The API equivalent is the `templates` field of `POST /api/experimental/organizations/{organization}/chats/projects` and `PATCH /api/experimental/organizations/{organization}/chats/projects/{project}`.
A `PATCH` that includes `templates` replaces the templates and all defaults, and a `PATCH` without it keeps them.

### Default template

Optionally, choose a **Default template** from the project's templates.
`list_templates` recommends the default template to the agent, unless the agent's search matches another template more closely.

For the default template, you can also set default values as either a **Preset** or **Parameter values**, but not both:

- **Preset:** when the agent creates a workspace from the default template without choosing a preset, Coder uses the default preset.
  Coder skips the default preset when the agent sets a parameter that the preset also sets.
- **Parameter values:** when the agent creates a workspace from the default template, Coder fills in each default value that the agent doesn't set.
  Values the agent sets take precedence.
  In the dialog, leave a value empty to use the template's own default.
  A value saved as empty through the API shows as **Empty value**, and **Use template default** removes it.

When the agent chooses a preset itself, the project's default values don't apply.

The default preset must belong to the template's active version.
After a new template version becomes active, chats skip a default preset from an older version.
The **Edit templates** dialog warns you about it: choose a preset from the active version, or save to clear it.

For a template in [parameter compatibility mode](../templates/extending-templates/parameters/compatibility-mode.md), Coder checks the default parameter values when you save.
When you edit the templates, the dialog warns about values for parameters that the active version no longer has, and saving removes them.
For other templates, Coder keeps every value and checks it when the agent creates a workspace.

The API keeps returning a default preset after it leaves the active version.
Sending those `templates` back unchanged fails with a `400` error until you set `default_preset_id` to a preset from the active version or remove it.
The same applies to a default value for a parameter that the active version of a template in compatibility mode no longer has.

### Template changes and existing workspaces

A change to a project's templates applies to its chats right away, including chats that are already running.
A chat that already has a workspace keeps it, even when the workspace's template isn't one of the project's templates.
This applies when you remove a template from the project and when you move a chat that has a workspace into the project.
