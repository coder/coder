---
title: Projects
---

> [!NOTE]
> Projects are experimental. Behavior and APIs may change between releases.
> Pin a release before broad rollout and review the release notes before
> upgrading.

Projects group related Coder Agents chats and give them shared, durable
memory.

## Enable the experiment

```sh
coder server --experiments=chat-projects
```

Or set the environment variable:

```sh
CODER_EXPERIMENTS=chat-projects
```

## Ownership

- Each project belongs to one organization and is owned by the user who
  created it. You pick the organization when you create the project, and it
  cannot be changed later.
- Projects are private to their owner. Other organization members cannot
  see or use a project or its memories. Sharing a chat in a project with
  [chat sharing](./chat-sharing.md) does not share the project.
- Any organization member who can use Coder Agents can create projects in
  that organization.
- Each user can own up to 100 projects across all organizations.
- Creating, updating, and deleting projects and memories is recorded in the
  [audit log](../../admin/security/audit-logs.md).

## Create and use a project

1. On the **Agents** page, select **New project** in the **Projects** section
   of the sidebar.
1. Enter a name, an optional description, and an icon, then choose the
   organization.
1. Open the project and start a chat from its page.

Chats started from a project page join that project and use the project's
organization. A chat's project is set when the chat is created and cannot be
changed afterward.

Edit or delete a project from its page or from the project's menu in the
sidebar. Deleting a project deletes its memories. Its chats are kept and no
longer belong to a project.

## Memory

Each project has a set of memories: short, named notes that persist across
every chat in the project. The agent manages them itself with four tools:

| Tool                 | Purpose                                                                                |
|----------------------|----------------------------------------------------------------------------------------|
| `read_memory`        | Read a memory's full body.                                                             |
| `save_memory`        | Save a new memory.                                                                     |
| `delete_memory`      | Delete a memory.                                                                       |
| `consolidate_memory` | Delete and save several memories in one atomic step, for merging or replacing entries. |

The agent saves facts that matter in future chats, such as people and their
preferences, corrections, decisions, deadlines, and where to find information
outside the codebase. It does not save anything it can derive from the code or
Git history. You can also ask it to remember or forget something.

How memory reaches the agent:

- Every chat in the project sees an index of memory names and one-line
  descriptions. The agent reads a full memory only when it needs it.
- Changes made in one chat appear in other chats at the start of their next
  turn.
- Only top-level chats get the memory tools. Sub-agents do not read or write
  memory.
- Memories are immutable. To change one, the agent replaces it.

Limits:

| Limit                | Value                                                                   |
|----------------------|-------------------------------------------------------------------------|
| Memories per project | 200. The agent is prompted to consolidate as the project nears the cap. |
| Name                 | Lowercase letters, digits, `-`, and `_`; up to 64 characters.           |
| Description          | 150 characters.                                                         |
| Body                 | 8&nbsp;KB.                                                              |

Memory changes the agent makes are audited as the chat owner, with the chat
ID attached.

There is no memory view in the dashboard yet. List, create, read, and delete
memories through the experimental API at
`/api/experimental/organizations/{organization}/chats/projects/{project}/memories`.
