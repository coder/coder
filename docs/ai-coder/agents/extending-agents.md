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

Place skill directories under `.agents/skills/` (project-scoped, relative
to the workspace working directory) or `~/.coder/skills/` (user-scoped, in
the home directory of the user running the agent). Each directory contains
a required `SKILL.md` file and any supporting files the skill needs.

On the first turn of a workspace-attached chat, the agent scans both
locations in order (`~/.coder/skills/` first, then `.agents/skills/`) and
builds an `<available-skills>` block in its system prompt listing each
skill's name and description. If the same skill name appears in both
locations, the copy in `~/.coder/skills/` wins, which lets developers keep
user-specific skills on the workspace without committing them to the repo.

Only frontmatter is read during discovery. The full skill content is
loaded lazily when the agent calls a tool.

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

### Environment variables

Override the default discovery paths and meta filename with environment
variables on the workspace. Typically these are set in the workspace
template, container image, or dotfiles:

| Variable                          | Default                          | Description                                                                                                          |
|-----------------------------------|----------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `CODER_AGENT_EXP_SKILLS_DIRS`     | `~/.coder/skills,.agents/skills` | Comma-separated list of directories to search for skills. Earlier entries override later entries on name collisions. |
| `CODER_AGENT_EXP_SKILL_META_FILE` | `SKILL.md`                       | Filename to look for inside each skill directory.                                                                    |

### Claude Code interoperability

The `SKILL.md` format the agent expects follows the same Agent Skills
standard as [Claude Code skills](https://docs.claude.com/en/docs/claude-code/skills):
kebab-case directory name, YAML frontmatter with `name` and `description`,
free-form markdown body. Only the discovery path differs. Claude Code
looks under `.claude/skills/` and `~/.claude/skills/`, while Coder looks
under `.agents/skills/` and `~/.coder/skills/` by default.

To share a skill with both runtimes, either:

- Symlink `.agents/skills` to `.claude/skills` in the workspace.
- Set `CODER_AGENT_EXP_SKILLS_DIRS=.claude/skills,~/.claude/skills` to
  point the Coder agent at the Claude Code paths.

## Workspace MCP tools

Workspace templates can expose custom
[MCP](https://modelcontextprotocol.io/introduction) tools by placing a
`.mcp.json` file in the workspace working directory. The agent discovers
these tools automatically when it connects to a workspace and registers
them alongside its built-in tools.

For deployment-level MCP servers that admins register once for the whole
deployment instead of per workspace, see
[MCP Servers](./platform-controls/mcp-servers.md) under platform controls.

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

`env` values in stdio configs support `${VAR}` substitution from the
agent's process environment, so secrets can live in the workspace template
or dotfiles rather than the committed `.mcp.json`. References to unset
variables expand to the empty string.

Server names (the keys under `mcpServers`) are used as a prefix for tool
names. They cannot contain `__` (the reserved tool-name separator) and
cannot start or end with an underscore. A single invalid server name
causes the entire config file to be skipped.

### Environment variables

Override the default config file location with an environment variable on
the workspace. Typically these are set in the workspace template,
container image, or dotfiles:

| Variable                           | Default     | Description                                                                                                                                                                                         |
|------------------------------------|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `CODER_AGENT_EXP_MCP_CONFIG_FILES` | `.mcp.json` | Comma-separated list of MCP config files to read. Entries are resolved relative to the workspace working directory. When the same server name appears in multiple files, the first occurrence wins. |

### How discovery works

The agent reads its MCP config files via the workspace agent connection on
each chat turn and reuses connected servers across turns. Edits to a
config file trigger an automatic reload (the agent watches the parent
directories via fsnotify, debounced by 250 ms), so you do not need to
restart the chat after changing `.mcp.json`. Empty results are not cached,
because the MCP servers may still be starting up.

Per-server connect attempts are bounded by a 30-second timeout. Servers
that fail to respond within that budget are skipped, and partial success
is acceptable.

### Tool naming

Tool names are prefixed with the server name as `serverName__toolName` to
avoid collisions between servers and with built-in tools.

### Timeouts

- **Discovery**: 35 seconds per chat turn.
- **Per-server connect**: 30 seconds.
- **Tool calls**: 60 seconds per invocation.
- **Watcher debounce**: 250 ms after a config file change.
