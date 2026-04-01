# Extending Agents

Workspace templates can extend the agent with custom skills and MCP tools.
These mechanisms let platform teams provide repository-specific instructions,
domain expertise, and external tool integrations without modifying the agent
itself.

## Skills

Skills are structured, reusable instruction sets that the agent loads on
demand. They live in the workspace filesystem and are discovered
automatically when a chat attaches to a workspace.

### How skills work

Place skill directories under `.agents/skills/` relative to the workspace
working directory. Each directory contains a required `SKILL.md` file and
any supporting files the skill needs.

On the first turn of a workspace-attached chat, the agent scans
`.agents/skills/` and builds an `<available-skills>` block in its system
prompt listing each skill's name and description. Only frontmatter is read
during discovery — the full skill content is loaded lazily when the agent
calls a tool.

Two tools are registered when skills are present:

| Tool              | Parameters                       | Description                                              |
|-------------------|----------------------------------|----------------------------------------------------------|
| `read_skill`      | `name` (string)                  | Returns the SKILL.md body and a list of supporting files |
| `read_skill_file` | `name` (string), `path` (string) | Returns the content of a supporting file                 |

### Directory structure

```text
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

```markdown
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
- `SKILL.md` has a maximum size of 64 KB.
- Supporting files have a maximum size of 512 KB. Files exceeding the limit
  are silently truncated.

### Path safety

`read_skill_file` rejects absolute paths, paths containing `..`, and
references to hidden files. All paths are resolved relative to the skill
directory.

## Workspace MCP tools

Workspace templates can expose custom
[MCP](https://modelcontextprotocol.io/introduction) tools by placing a
`.mcp.json` file in the workspace working directory. The agent discovers
these tools automatically when it connects to a workspace and registers
them alongside its built-in tools.

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

**Stdio transport** — set `command`, and optionally `args` and `env`. The
agent spawns the process in the workspace.

**HTTP transport** — set `url`, and optionally `headers`. The agent connects
to the HTTP endpoint from the workspace.

### How discovery works

The agent reads `.mcp.json` via the workspace agent connection on each chat
turn. Discovery uses a 5-second timeout. Servers that fail to
respond are skipped — partial success is acceptable. Empty results are not
cached because the MCP servers may still be starting.

### Tool naming

Tool names are prefixed with the server name as `serverName__toolName` to
avoid collisions between servers and with built-in tools.

### Timeouts

- **Discovery**: 5-second timeout.
- **Tool calls**: 60 seconds per invocation.
