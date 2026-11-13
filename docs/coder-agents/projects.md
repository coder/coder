---
title: Projects
---

> [!NOTE]
> Projects are in [early access](../reference/feature-stages.md#early-access-features).
> Behavior and APIs might change between releases.

Projects group related Coder Agents chats.
Every chat in a project gets the project's instructions, skills, and memory, so you don't repeat the same context in each chat.
Select **Projects** below **New chat** in the **Agents** page sidebar to see your projects as cards.
The **Your projects** tab lists the projects you own.
The **Shared with you** tab lists projects that other users own and you can use.
While you have [archived projects](#archive-a-project), an **Archived** tab lists them.
Each card shows whether one of your chats in the project needs attention or is running, or else how many of your chats are unread or read.
It also shows how many users can use the project, counting its owner, and when your chats in it last changed.
[Pinned projects](#pin-a-project) also appear in the sidebar.

## What a project provides

| Capability                    | What it does                                                             |
|-------------------------------|--------------------------------------------------------------------------|
| [Instructions](#instructions) | Text added to the system prompt of every chat in the project.            |
| [Skills](#skills)             | Skills available only to chats in the project.                           |
| [Memory](#memory)             | Durable notes that the agent saves in one chat and reads in later chats. |

## Ownership

Each project belongs to one organization and is owned by the user who created it.
You choose the organization when you create the project, and you can't change it later.
Chats in a project must be in the project's organization.

Any organization member who can use Coder Agents can create projects in that organization.
Each user can own up to 100 projects across all organizations.

Only the owner can use a project until they share it.
Project sharing is experimental: your deployment administrator must include `chat-project-sharing` in the `CODER_EXPERIMENTS` environment variable or the `coder server --experiments` list.
The owner can then share the project with users and groups in its organization, and it appears on the **Shared with you** tab for them.
If your administrator later removes the experiment, owners can no longer change who a project is shared with, but existing shares keep working.
[Disabling chat sharing](./chat-sharing.md#disable-chat-sharing) also turns off project sharing.
Coder records project, instruction, skill, and memory changes in the [audit log](../admin/security/audit-logs.md).

## Create a project

1. On the **Projects** page, select **New project**.
1. Enter a **Project name** and an optional **Description**, and pick an icon.
1. Choose the **Organization**.
1. Select **Create project**.

New projects start pinned to your sidebar.
The project page shows a **Details** panel with the project owner, creation date, organization, skills, and instructions, and lists your chats in the project.

To rename or delete a project, open the project's menu on its page, on its card, or in the sidebar and select **Edit project** or **Delete project**.
Deleting a project deletes its instructions, skills, and memories.
Its chats are kept and move back to your chat list.
To set a project aside without deleting it, [archive it](#archive-a-project).

## Archive a project

Archive a project when its work is done.
The project owner, organization admins, and site Owners can archive and unarchive a project.
A project that isn't shared with you doesn't appear on the **Projects** page, so admins open it from its link and use the menu or banner on its page.
Admins can't start chats in a project that isn't shared with them.

To archive a project, open the project's menu on its page, on its card, or in the sidebar and select **Archive project**.
The dialog shows how many top-level chats will be archived, along with their subagent chats, and what happens next:

- Every chat in the project is archived, including chats that other users started in it.
- If chats are active (running, interrupting, waiting on a tool result, or paused on a queued message being edited), the dialog warns you and the button changes to **Stop chats and archive**, or **Stop chat and archive** when the active chats all belong to one top-level chat.
  Archiving stops them and discards any reply in progress.
  Queued messages are kept.
- If your own chats in the project use workspaces, choose whether to leave, stop, or delete those workspaces.
  The dialog marks workspaces that existed before the chat that uses them.
  Deleting workspaces requires you to type the project name.
  A workspace that an unarchived chat outside the project still uses is skipped.

Archived chats follow the deployment's [conversation data retention](./platform-controls/chat-retention.md) period.
After that period, Coder permanently deletes them, and unarchiving the project doesn't bring them back.
When retention is turned off, archived chats are kept until you unarchive the project.

An archived project leaves the sidebar, the **Your projects** and **Shared with you** tabs, the new-chat project picker, and **Move to project**, and you can't pin it.
Its page stays available to read its archived chats, but you can't start chats in it, move chats into it, or edit its details, instructions, skills, sharing, or memory.

To unarchive a project, open its card's menu on the **Archived** tab of the **Projects** page and select **Unarchive project**, or select **Unarchive** on the project's page.
Unarchiving restores the chats that were archived with the project, and a project you had pinned returns to the **Pinned** area.
You can't unarchive a single chat while its project is archived.
Unarchive the project instead.

The API equivalents are `POST /api/experimental/organizations/{organization}/chats/projects/{project}/archive` and `POST /api/experimental/organizations/{organization}/chats/projects/{project}/unarchive`.
When any chat in the project, or one of its subagent chats, is active (`running`, `interrupting`, `requires_action`, or `paused`), the archive request returns `409 Conflict` unless its body is `{"stop_active_chats": true}`.
List archived projects with `GET /api/experimental/chats/projects?archived=true`.

## Add chats to a project

- Start a chat from the project page.
  The composer is locked to the project's organization.
- In the general new-chat composer, choose the project in the project picker.
  The picker defaults to **No project**.
- Move an existing chat: open the chat's menu, select **Move to project**, and choose a project.
  Select **Remove from project** to take it out.
- Drag a chat in the sidebar onto a pinned project.
  Only pinned projects appear in the sidebar, so use **Move to project** to move a chat into an unpinned project.
  If that move takes the chat out of the sidebar, a dialog explains that the sidebar lists the project's chats only while the project is pinned, and offers **Pin project** and **Go to project**.
  This happens when the chat isn't pinned and was listed in the sidebar before the move.

You can move only top-level chats that you own, that aren't archived, and that aren't running.
Sub-agent chats always follow their top-level chat.
After a move, the chat's next turn uses the new project's instructions, skills, and memory.

The API equivalent is `PATCH /api/v2/chats/{chat}` with `project_id`.
Set `project_id` to the nil universally unique identifier (UUID) (`00000000-0000-0000-0000-000000000000`) to remove a chat from its project.

## Pin a project

- To pin a project, open its card's menu on the **Projects** page and select **Pin project**.
  The project appears at the end of the **Pinned** area of the sidebar, below your pinned chats.
- To unpin a project, select the pin icon on its card.
- You can also open the project's menu on its card or in the sidebar and select **Unpin project**.
- To reorder pinned projects, drag them in the **Pinned** area of the sidebar.

Pins are personal and change only your own sidebar.
Projects you create start pinned at the end of the **Pinned** area, and projects that other users share with you start unpinned.
When you upgrade to a release with pinning, Coder pins every project you already own, so your sidebar doesn't lose any projects.

The sidebar lists a chat in an unpinned project only if you pin the chat itself.
Every chat stays available from the project's page.
Chats you own in a project that you can no longer open, for example after the owner stops sharing it, stay in the sidebar.

The API equivalents are `PUT` and `DELETE` on `/api/experimental/organizations/{organization}/chats/projects/{project}/pin`.

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
