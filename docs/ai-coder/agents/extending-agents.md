# Extending Agents

Workspace templates can extend the agent with custom skills and MCP tools.
These mechanisms let platform teams provide repository-specific instructions,
domain expertise, and external tool integrations without modifying the agent
itself.

## How the workspace shares context with chats

The workspace agent owns discovery.
It scans a fixed set of locations for instruction files, skills, and MCP configuration, then pushes the result to Coder as a single context snapshot.
Chats never scan the workspace themselves; they read the snapshot the agent pushed.

A workspace-attached chat pins one snapshot.
The pinned copy is what the agent's system prompt and tool list are built from, so a chat keeps working with a consistent view of your skills and MCP tools even while you edit files in the workspace.

The lifecycle of an edit looks like this:

1. You add a skill, edit `.mcp.json`, or change an instruction file in the workspace.
1. A file watcher notices the change, and the agent re-scans after a short period and then pushes a new snapshot.
1. Chats that have not pinned a snapshot yet pin the new one immediately.
1. Chats that already pinned an older snapshot are marked out of date instead of being switched over.
1. Selecting **Refresh context** in the chat re-pins that chat to the latest snapshot.

This is why an in-flight chat can keep using an older snapshot: a push never rewrites a chat's pinned context.
The pin only moves when the chat is first hydrated or when you refresh it.

The agent publishes nothing until the workspace is ready.
Until startup scripts finish, the agent holds an empty snapshot, so a chat opened during workspace startup can show no skills and no MCP tools until the first real push lands.

### Where the agent looks

Each scan location is a scan root.
The agent scans the following:

- The workspace working directory
- `~/.coder` and `~/.coder/skills`
- `~/.claude/plugins/cache`
- Any additional source you declare through the agent's context sources API

Declared sources contribute instruction files and skills only.
A `.mcp.json` file inside a declared source appears in the context inventory, but the agent does not connect to the servers it lists; MCP servers load only from the workspace working directory.

Discovery is shallow.
Instruction files (`AGENTS.md`, `CLAUDE.md`, `.cursorrules`) and `.mcp.json` are read only at the top level of a scan root, and skills are read only from the fixed container directories described below.
The agent never walks further down the tree and never climbs to a parent directory.
To pick up context in another directory, declare that directory as its own source.

### Snapshot limits

A snapshot is capped so a large workspace cannot flood a chat's prompt:

| Limit                                        | Behavior when exceeded                                                                                   |
|----------------------------------------------|----------------------------------------------------------------------------------------------------------|
| 64&nbsp;KiB per file resource                | The resource is reported with an oversize status and no content, so the chat lists it but cannot read it |
| 2&nbsp;MiB of file content across a snapshot | Resources past the cap are reported as excluded with no content                                          |
| 500 resources per snapshot                   | Resources past the cap are reported as excluded with no content                                          |

The byte caps measure file contents, such as instruction files and skill files.
MCP server entries count toward the 500-resource cap, but their tool definitions are not measured by the byte caps.

Excluded and oversize resources still appear in the chat's context list with their status and error, so a dropped skill or instruction file is visible rather than silently missing.

## Skills

Skills are structured, reusable instruction sets that the agent loads on
demand. They live in the workspace filesystem and are discovered
automatically when a chat attaches to a workspace.

### How skills work

A skill is a directory containing a required `SKILL.md` file and any supporting files the skill needs.
The agent looks for skill directories in the immediate children of these containers, relative to each scan root:

- `skills/`
- `.agents/skills/`
- `.claude/skills/`
- `.codex/skills/`

Because `~/.coder/skills` is itself a scan root, skills placed directly under it are discovered too.

Each discovered skill contributes its name and description to the `<available-skills>` block in the agent's system prompt.
The full instructions are loaded only when the agent calls a tool, and they are served from the chat's pinned snapshot rather than read live from the workspace.
A skill added after a chat pinned its snapshot appears in that chat after you refresh its context.

Two tools are registered when skills are present:

| Tool              | Parameters                       | Description                                                                                                |
|-------------------|----------------------------------|------------------------------------------------------------------------------------------------------------|
| `read_skill`      | `name` (string)                  | Returns the SKILL.md body, the absolute skill directory (workspace skills), and a list of supporting files |
| `read_skill_file` | `name` (string), `path` (string) | Returns the content of a supporting file                                                                   |

For workspace skills, `read_skill` also returns `dir`, the absolute path to
the skill directory in the workspace. The agent's `read_file` and `execute`
tools operate on that same workspace filesystem, so you can join `dir` with a
supporting file's relative path to read or run that file directly, for example
to execute a bundled `scripts/` helper. `read_skill_file` remains available as
a path-safe convenience for reading supporting files.

Supporting files are read over the workspace connection, so `read_skill_file` and the supporting-file list need a reachable workspace.
The skill body comes from the pin and keeps working when the workspace is unreachable.

### Directory structure

```txt
.agents/skills/
├── deep-review/
│   ├── SKILL.md
│   └── roles/
│       ├── security-reviewer.md
│       └── concurrency-reviewer.md
├── pull-requests/
│   └── SKILL.md
└── refine-plan/
    └── SKILL.md
```

### SKILL.md format

Each `SKILL.md` starts with YAML frontmatter containing a `name` and an
optional `description`, followed by the full instructions in markdown:

```md
---
name: deep-review
description: "Multi-reviewer code review with domain-specific reviewers"
---

# Deep Review

Instructions for the skill go here...
```

### Naming and size constraints

- Names must be kebab-case (`^[a-z0-9]+(-[a-z0-9]+)*$`) and match the
  directory name exactly.
- `SKILL.md` has a maximum size of 64&nbsp;KB.
  A larger file is reported with an oversize status and contributes no content to the chat.
- Supporting files have a maximum size of 512&nbsp;KB.
  Files exceeding the limit are silently truncated.

A skill with its frontmatter missing, unparsable, not kebab-case, or not matching its directory name is reported as invalid and contributes no instructions.

### Path safety

`read_skill_file` rejects absolute paths, paths containing `..`, and
references to hidden files. All paths are resolved relative to the skill
directory.

## Personal skills

Personal skills are user-owned skills that are available to all of your
chats. They are not tied to a specific workspace. Manage them from the
**Agents** page, under **Settings** > **Personal Skills**.

Personal skills use the same `SKILL.md` format as workspace skills: YAML
frontmatter with a kebab-case `name`, an optional `description`, and a
markdown body. This keeps content portable between personal skills and
workspace skills.

```md
---
name: personal-reviewer
description: "Personal review guidance"
---

# Personal Reviewer

Instructions for the skill go here...
```

Each personal skill is stored as a single `SKILL.md` file containing
frontmatter and body content. Supporting files are not supported. Each
`SKILL.md` file can be up to 64&nbsp;KB, and each user can create up to 100 personal skills.

Personal skills are stored in Coder, not in the workspace, so they are not part of a workspace context snapshot and do not depend on a pin or a refresh.

If you need richer skills with supporting files or multiple files, use
workspace skills instead. Store them in the repo under
`.agents/skills/<name>/`, or load them from a workspace.

## Workspace MCP tools

Workspace templates can expose custom
[MCP](https://modelcontextprotocol.io/introduction) tools by placing a
`.mcp.json` file in the workspace working directory.
The agent connects to these servers, reports their tools in the context snapshot it pushes, and chats register those tools alongside their built-in tools.

### Configuration

Define MCP servers in `.mcp.json` at the workspace root. Each entry under
`mcpServers` describes a server. The transport type is inferred from
whether `command` or `url` is present, or you can set it explicitly with
`type`:

```json
{
  "mcpServers": {
    "github": {
      "command": "github-mcp-server",
      "args": ["--token", "..."]
    },
    "my-api": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": { "Authorization": "Bearer ..." }
    }
  }
}
```

**Stdio transport**: set `command`, and optionally `args` and `env`. The
agent spawns the process in the workspace.

**HTTP transport**: set `url`, and optionally `headers`. The agent connects
to the HTTP endpoint from the workspace.

### How discovery works

The agent connects to the servers declared in `.mcp.json` once startup scripts finish.
Servers that fail to connect are skipped, and the rest still contribute their tools.

A single set of connections is shared by tool discovery and tool execution, so each declared server is launched once.
When the connected tool list changes, the agent re-scans and pushes a new snapshot.

Editing `.mcp.json` does not require a workspace restart.
The agent notices edits to the file and reloads its servers automatically.
The reload changes the pushed snapshot, but an MCP-only change does not mark existing chats out of date, and the dashboard offers **Refresh context** only on chats that are marked out of date.
New chats and chats that have not pinned a snapshot yet receive the new tool set immediately.
An existing chat keeps its current tool set until another context change, such as editing an instruction file or a skill, marks it out of date; refreshing then re-pins the whole snapshot, including the new MCP tools.

The snapshot carries tool definitions only, not a way to run them.
Every workspace MCP tool call is proxied back through the workspace agent, so a chat can list workspace MCP tools while the workspace is unreachable, but calling one requires a running workspace with the server connected.

### Tool naming

Tool names are prefixed with the server name as `serverName__toolName` to
avoid collisions between servers and with built-in tools.

### Timeouts

- **Server connect**: 30&nbsp;seconds per server.
- **Tool calls**: 60&nbsp;seconds per invocation.
