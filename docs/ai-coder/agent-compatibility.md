# Agent Compatibility

Coder Tasks works with a range of AI coding agents, each with different levels
of support for preserving conversation context across workspace restarts. This
page covers which agents support pause and resume, what session data they store,
and what to watch out for when configuring persistent storage.

## Compatibility levels

Agents with **full support** automatically resume the previous session when a
workspace restarts. The conversation history, tool calls, and context are all
preserved, so the agent picks up exactly where it left off. Agents with
**partial support** cannot resume a previous session and start a fresh
conversation on each restart, even if some chat history is visible in the UI.

## Compatibility matrix

| Agent           | Module                                                                           | Support | Session data paths                                   | Min storage               |
|-----------------|----------------------------------------------------------------------------------|---------|------------------------------------------------------|---------------------------|
| Claude Code     | [claude-code](https://registry.coder.com/modules/coder/claude-code)              | Full    | `~/.claude/`                                         | 100 MB (can grow to GB)   |
| Codex           | [codex](https://registry.coder.com/modules/coder-labs/codex)                     | Full    | `~/.codex/`, `~/.codex-module/`                      | 100 MB                    |
| Copilot         | [copilot](https://registry.coder.com/modules/coder-labs/copilot)                 | Full    | `~/.copilot/`                                        | 50 MB                     |
| OpenCode        | [opencode](https://registry.coder.com/modules/coder-labs/opencode)               | Full    | `~/.local/share/opencode/`, `~/.config/opencode/`    | 50 MB                     |
| Auggie          | [auggie](https://registry.coder.com/modules/coder-labs/auggie)                   | Full    | `~/.augment/`                                        | 50 MB                     |
| Goose           | [goose](https://registry.coder.com/modules/coder/goose)                          | Full    | `~/.local/share/goose/sessions/`, `~/.config/goose/` | 50 MB                     |
| Amazon Q        | [amazon-q](https://registry.coder.com/modules/coder/amazon-q)                    | Full    | `~/.local/share/amazon-q/`, `~/.aws/amazonq/`        | 50 MB                     |
| Gemini          | [gemini](https://registry.coder.com/modules/coder-labs/gemini)                   | Full    | `~/.gemini/`                                         | 200 MB (can reach 400 MB) |
| Cursor CLI      | [cursor-cli](https://registry.coder.com/modules/coder-labs/cursor-cli)           | Full    | `~/.cursor/`                                         | 50 MB                     |
| Sourcegraph Amp | [sourcegraph-amp](https://registry.coder.com/modules/coder-labs/sourcegraph-amp) | Full    | `~/.config/amp/` (config only)                       | 10 MB                     |
| Aider           | [aider](https://registry.coder.com/modules/coder/aider)                          | Partial | `.aider.chat.history.md` (workdir)                   | 50 MB                     |

## Persistent storage

Every agent's session data lives under the home directory, so persisting the
home directory with a volume mount is the simplest way to cover all agents at
once. This also preserves the AgentAPI state file that Coder uses to stream chat
content between the agent and the Tasks UI.

For instructions on configuring Docker volumes and Kubernetes PVCs that survive
workspace restarts, see
[Resource persistence](../admin/templates/extending-templates/resource-persistence.md).
The key patterns are using `lifecycle { ignore_changes = all }` to prevent
Terraform from recreating volumes on template updates and keying volume names on
`coder_workspace.me.id` so each workspace gets its own persistent storage.

## Agent-specific notes

**Claude Code** -- Session files are JSONL and grow unbounded. Long-running
workspaces can accumulate multiple gigabytes of data in `~/.claude/projects/`.
Monitor disk usage and consider periodic cleanup.

**Goose** -- Sessions are stored in a SQLite database with WAL mode enabled. You
must preserve the `-wal` and `-shm` sidecar files alongside the main database,
or the session database may become corrupted.

**Amazon Q** -- The Amazon Q Developer CLI has been rebranded to Kiro CLI. The
existing module pins a specific CLI version. An authentication tarball is stored
alongside session data; if it is lost, the agent must re-authenticate.

**Gemini** -- Session data can reach 400 MB for long-running tasks. You can set
the `general.sessionRetention` configuration value to control how long sessions
are retained.

**Sourcegraph Amp** -- Conversation threads are stored server-side on
Sourcegraph servers, so only local configuration in `~/.config/amp/` needs
persistence. The workspace must have network connectivity to Sourcegraph for
resume to work.

**Auggie** -- May require connectivity to the Augment cloud backend for session
resume. Behavior in fully headless or network-restricted environments is not
fully verified.

**Aider** -- The `--restore-chat-history` flag performs a lossy reconstruction
from a Markdown log file, but the agent loses full conversation context on each
restart and does not support MCP for status reporting. When
`enable_state_persistence` is enabled in the module, the Coder UI preserves chat
history across restarts, but Aider itself starts each session fresh with no
memory of previous conversations.

## Next steps

- [Set up Coder Tasks](./tasks.md) in your template.
- [Build a custom agent](./custom-agents.md) with MCP support.
