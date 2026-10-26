---
title: Projects
---

> [!NOTE]
> Projects are experimental.
> Behavior and APIs might change between releases.
> Pin a release before broad rollout and review the release notes before upgrading.

Projects group related Coder Agents chats.
Every chat in a project gets the project's instructions, skills, and memory, so you don't repeat the same context in each chat.

## Enable the experiment

```sh
coder server --experiments=chat-projects
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=chat-projects
```

When the experiment is on, the **Agents** page sidebar shows a **Projects** section.

## What a project provides

| Capability                    | What it does                                                                                          |
|-------------------------------|-------------------------------------------------------------------------------------------------------|
| [Instructions](#instructions) | Text added to the system prompt of every chat in the project.                                         |
| [Skills](#skills)             | Skills available only to chats in the project.                                                        |
| [Memory](#memory)             | Durable notes that the agent saves in one chat and reads in later chats.                              |
| [Sharing](#share-a-project)   | Lets other organization members start their own chats with the same instructions, skills, and memory. |

## Ownership

Each project belongs to one organization and is owned by the user who created it.
You choose the organization when you create the project, and you can't change it later.
Chats in a project must be in the project's organization.

Any organization member who can use Coder Agents can create projects in that organization.
Each user can own up to 100 projects across all organizations.

A project is private to its owner until the owner [shares](#share-a-project) it.
Coder records project, instruction, skill, memory, and sharing changes in the [audit log](../../admin/security/audit-logs.md).

## Create a project

1. On the **Agents** page, select **New project** in the **Projects** section of the sidebar.
1. Enter a **Project name** and an optional **Description**, and pick an icon.
1. Choose the **Organization**.
1. Select **Create project**.

The project page shows a **Details** panel with the project owner, creation date, organization, skills, and instructions, and lists your chats in the project.

To rename or delete a project, open the project's menu on its page or in the sidebar and select **Edit project** or **Delete project**.
Deleting a project deletes its instructions, skills, and memories.
Its chats are kept and move back to each owner's chat list.

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

The API equivalent is `PATCH /api/v2/chats/{chat}` with `project_id`.
Set `project_id` to the nil UUID (`00000000-0000-0000-0000-000000000000`) to remove a chat from its project.

## Instructions

Project instructions are added to the system prompt of every chat in the project, including sub-agent chats, for every user.
Use them for conventions, context, and preferences that apply to all work in the project.

To add or change them, select **Create** or the edit icon under **Instructions** in the project's **Details** panel.
Chats read instructions on every turn, so a change takes effect on each chat's next turn.

Instructions share the deployment's prompt size limit, set with `--chat-max-prompt-bytes` (default 128&nbsp;KiB).

## Skills

Project skills are single-file `SKILL.md` skills that only chats in the project can load.
Manage them by selecting the **Skills** row in the project's **Details** panel.

A project skill can show as a one-click action above the project's chat composers, or be limited to manual use from the `/` menu.
For the format, limits, and how skills from different sources resolve, see [Project skills](./extending-agents.md#project-skills).

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
Everyone with access to the project can read and change its memories, so the agent is told not to save secrets, credentials, or personal details.
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

## Share a project

A project owner can share a project with organization members, groups, or the whole organization.
Sharing a project doesn't share its chats.
Each person's chats in the project stay private to them; use [chat sharing](./chat-sharing.md) to share a chat.

| Role    | Start chats in the project | Read and write memories | Edit the project, instructions, and skills | Share the project | Delete the project |
|---------|----------------------------|-------------------------|--------------------------------------------|-------------------|--------------------|
| `use`   | Yes                        | Yes                     | No                                         | No                | No                 |
| `admin` | Yes                        | Yes                     | Yes                                        | Yes               | No                 |

Only the owner, and administrators whose role grants delete, can delete a project.
Projects shared with you appear in your sidebar next to your own projects.

The dashboard doesn't have a sharing control yet.
Manage sharing with the experimental API:

- `GET /api/experimental/organizations/{organization}/chats/projects/{project}/acl` lists who has access.
- `PATCH` on the same path sets roles.
  The body takes `user_roles` and `group_roles` maps from ID to `use`, `admin`, or `""` to remove access.
  Use the organization ID as the group ID to share with everyone in the organization.

When access is removed, the person's existing chats in the project lose its instructions, skills, and memory on their next turn.
Removing someone from the organization removes their project access, and adding them back doesn't restore it.

`--disable-chat-sharing` also turns off project sharing.
While it's set, existing project shares are ignored and new ones are rejected.
