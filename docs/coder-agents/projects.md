---
title: Projects
---

> [!NOTE]
> Projects are in [early access](../reference/feature-stages.md#early-access-features).
> Behavior and APIs might change between releases.

Projects group related Coder Agents chats.
Every chat in a project gets the project's instructions, skills, and memory, so you don't repeat the same context in each chat.
You can also attach templates to a project, and limit its chats to those templates.
Your projects appear in the **Projects** section of the **Agents** page sidebar.

## What a project provides

| Capability                    | What it does                                                             |
|-------------------------------|--------------------------------------------------------------------------|
| [Instructions](#instructions) | Text added to the system prompt of every chat in the project.            |
| [Skills](#skills)             | Skills available only to chats in the project.                           |
| [Memory](#memory)             | Durable notes that the agent saves in one chat and reads in later chats. |
| [Templates](#templates)       | Templates the agent lists first, and an optional limit to only those.    |

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
1. Under **Templates**, select the templates to attach to the project.

   You can skip this step and attach templates later.
   To limit the project's chats to these templates, turn on **Limit chats to these templates**.
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
If the new project [limits its chats to its templates](#limit-chats-to-the-projects-templates), the chat can use only those templates, but it keeps any workspace it already has.

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

You can attach templates from the project's organization to a project.
In a project chat, `list_templates` lists the attached templates before other templates that match the agent's search equally well.
Attaching a template never gives anyone access to it.
The agent can use an attached template only when the chat's owner can use it.

Unless the project limits its chats to its templates, the agent can also use every other template that the chat's owner can use.

### Limit chats to the project's templates

A project doesn't limit which templates its chats use until you turn on the limit.
To turn it on, use **Limit chats to these templates** when you create the project or in the **Edit templates** dialog.
While the limit is on:

- `list_templates` returns only the attached templates that the chat's owner can use.
  `create_workspace` and `read_template` reject every other template.
- To give a project chat an existing workspace, the workspace must use one of those templates.
  This applies when you pick a workspace as you start the chat, where the composer lists only those workspaces, and when you set `workspace_id` through the API.
  The API rejects other workspaces with `This template is not one of the templates the chat's project allows.`
- When the project has no attached template that the chat's owner can use, the agent can't create a workspace, and you can't give the chat an existing workspace.
  If the agent in a project chat says that no templates are available, attach templates to the project or turn off the limit.

A chat that already has a workspace keeps it, as [Template changes and existing workspaces](#template-changes-and-existing-workspaces) describes.

### Change a project's templates

To change a project's templates, select the template count, such as **2 templates**, in the **Templates** row of the project's **Details** panel.
This opens the **Edit templates** dialog.
You need the `chat_project:update` permission on the project, which the project owner has.
To attach a template, you also need permission to use it.
To detach a template, you need permission to read it.

The dialog lists the templates in the project's organization that you can access, that allow Coder Agents, and that aren't deprecated.
**Select all** selects every template in the list.
When you save, the dialog attaches the templates you added and detaches the templates you removed.
It doesn't change what someone else attached or detached while you edited.
If a save fails partway, the dialog shows the error and the changes made before the failure stay saved.
Select **Save** again to save the rest.

While an attached template is deleted, deprecated, or doesn't allow Coder Agents, the project doesn't show it and its chats can't use it.
Attaching a template never overrides the administrator setting: the agent can't use a template that doesn't allow Coder Agents.
For that setting, refer to [Template routing](./platform-controls/index.md#template-routing).

The dialog checks for template changes made elsewhere, such as in another browser tab, when it opens, when you return to its browser tab, and again just before it saves.
If you haven't edited anything, the dialog switches to the newer templates.
If you have unsaved edits, it shows a warning and you can't save until you choose.
Select **Load latest** to start over from the newer templates, or **Keep my draft** to apply your changes to them.
The dialog also reloads the list of templates you can attach when you return to its tab.
If the check fails, or the list of templates or the default template's presets don't load, saving is paused until you select **Retry**.
The same applies to parameters of a template in compatibility mode.
If you edit the draft while the check runs, nothing is saved, so select **Save** again.
You can close the dialog while it checks, which cancels the save.

The API equivalents are:

| Action                                   | Request                                                                                                                             |
|------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| Create a project with templates          | `POST /api/experimental/organizations/{organization}/chats/projects` with `template_ids`, `limit_templates`, and `default_template` |
| Attach a template                        | `PUT /api/experimental/organizations/{organization}/chats/projects/{project}/templates/{template}`                                  |
| Detach a template                        | `DELETE /api/experimental/organizations/{organization}/chats/projects/{project}/templates/{template}`                               |
| Change the limit or the default template | `PATCH /api/experimental/organizations/{organization}/chats/projects/{project}` with `limit_templates` or `default_template`        |

Attaching a template that is already attached succeeds.
Detaching returns a `404` error when the template isn't attached or you can't read it.
A `PATCH` doesn't change which templates are attached.

### Templates you can't access

A project can have attached templates that you can't access, for example templates that someone else attached.
The **Details** panel and the **Edit templates** dialog show only how many there are, such as **2 hidden**.
The API returns only their number, in `hidden_template_count`.
A template admin or organization admin can give you access to a template.

The agent in your chats can't use these templates, and `list_templates` tells the agent that some of the project's templates aren't listed.
You can't detach a template that you can't access, and saving the **Edit templates** dialog never detaches one.
When you can't access the project's default template, the dialog and the API show no default template.

### Default template

Optionally, choose a **Default template** from the project's attached templates.
`list_templates` recommends the default template to the agent, unless the agent's search matches another template more closely.
The default template and its default values apply only while the template is attached.
Detaching the default template also removes the project's default template and its default values.

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
A value for a parameter that the active version no longer has isn't shown in the dialog, and the workspace build ignores it.
To remove it, update `default_template` through the API, or switch the default values to **Preset** and save, which clears every default parameter value.

In the API, `default_template` holds the `template_id` and either a `preset_id` or `parameters`.
A `PATCH` that includes `default_template` replaces all three, and a `PATCH` without it keeps them.
To clear the default template, set `template_id` to the nil UUID.

The API keeps returning a default preset after it leaves the active version.
Sending that `default_template` back unchanged fails with a `400` error until you set `preset_id` to a preset from the active version or remove it.
The same applies to a default value for a parameter that the active version of a template in compatibility mode no longer has.

### Template changes and existing workspaces

A change to a project's templates or its limit applies to its chats right away, including chats that are already running.
A chat that already has a workspace keeps it, even when the project limits its chats to templates that don't include the workspace's template.
This applies when you detach a template, when you turn on the limit, and when you move a chat that has a workspace into the project.
