# Agent compatibility

> [!WARNING]
> Starting June 2, 2026, Coder Tasks will move to a 12-month Extended Support Release (ESR) for Premium customers.
>
> Tasks will be removed from new Coder releases beginning with v2.37 (September 1, 2026) and will only be available via the ESR during the support period.
>
> We recommend transitioning to [Coder Agents](./agents/index.md), the long-term replacement.

Coder Tasks works with a range of AI coding agents, each with different levels
of support for preserving conversation context across pause and resume cycles.
This page covers which agents support resume, what session data they store,
and what to watch out for when configuring persistent storage.

## Compatibility levels

Agents with **full support** automatically resume the previous session when a
task resumes. The conversation history, tool calls, and context are all
preserved, so the agent picks up exactly where it left off.

Agents with **partial support** have resume wiring in the module but it is
either off by default or has known bugs. A module update is needed before resume
works reliably. See the linked tracking issue for details.

Agents with **planned support** have native session persistence but the registry
module does not wire it yet. These agents start a fresh conversation on each
resume until the module is updated.

Agents marked **not supported** cannot resume a previous session. They start a
fresh conversation on each resume, even if some chat history is visible in the
UI.

## Compatibility matrix

| Agent           | Module                                                                           | Min version | Support       | Tracking                                                     | Session data paths                                   | Min storage               |
|-----------------|----------------------------------------------------------------------------------|-------------|---------------|--------------------------------------------------------------|------------------------------------------------------|---------------------------|
| Claude Code     | [claude-code](https://registry.coder.com/modules/coder/claude-code)              | >= 4.8.0    | Full          | -                                                            | `~/.claude/`                                         | 100 MB (can grow to GB)   |
| Codex           | [codex](https://registry.coder.com/modules/coder-labs/codex)                     | >= 4.2.0    | Full          | -                                                            | `~/.codex/`, `~/.codex-module/`                      | 100 MB                    |
| Copilot         | [copilot](https://registry.coder.com/modules/coder-labs/copilot)                 | -           | Partial       | [registry#741](https://github.com/coder/registry/issues/741) | `~/.copilot/`                                        | 50 MB                     |
| OpenCode        | [opencode](https://registry.coder.com/modules/coder-labs/opencode)               | -           | Partial       | [registry#742](https://github.com/coder/registry/issues/742) | `~/.local/share/opencode/`, `~/.config/opencode/`    | 50 MB                     |
| Auggie          | [auggie](https://registry.coder.com/modules/coder-labs/auggie)                   | -           | Planned       | [registry#743](https://github.com/coder/registry/issues/743) | `~/.augment/`                                        | 50 MB                     |
| Goose           | [goose](https://registry.coder.com/modules/coder/goose)                          | -           | Planned       | [registry#744](https://github.com/coder/registry/issues/744) | `~/.local/share/goose/sessions/`, `~/.config/goose/` | 50 MB                     |
| Amazon Q        | [amazon-q](https://registry.coder.com/modules/coder/amazon-q)                    | -           | Planned       | [registry#746](https://github.com/coder/registry/issues/746) | `~/.local/share/amazon-q/`, `~/.aws/amazonq/`        | 50 MB                     |
| Gemini          | [gemini](https://registry.coder.com/modules/coder-labs/gemini)                   | -           | Planned       | [registry#745](https://github.com/coder/registry/issues/745) | `~/.gemini/`                                         | 200 MB (can reach 400 MB) |
| Cursor CLI      | [cursor-cli](https://registry.coder.com/modules/coder-labs/cursor-cli)           | -           | Planned       | [registry#747](https://github.com/coder/registry/issues/747) | `~/.cursor/`                                         | 50 MB                     |
| Sourcegraph Amp | [sourcegraph-amp](https://registry.coder.com/modules/coder-labs/sourcegraph-amp) | -           | Planned       | [registry#748](https://github.com/coder/registry/issues/748) | `~/.config/amp/` (config only)                       | 10 MB                     |
| Aider           | [aider](https://registry.coder.com/modules/coder/aider)                          | -           | Not supported | [registry#739](https://github.com/coder/registry/issues/739) | `.aider.chat.history.md` (workdir)                   | 50 MB                     |

## Persistent storage

Every agent's session data lives under the home directory, so persisting the
home directory with a volume mount is the simplest way to cover all agents at
once. This also preserves the AgentAPI state file that Coder uses to stream chat
content between the agent and the Tasks UI.

See
[Resource persistence](../admin/templates/extending-templates/resource-persistence.md)
for configuration patterns.

## Agent-specific notes

**Claude Code**: Session files are JSONL and grow unbounded. Long-running
tasks can accumulate multiple gigabytes of data in `~/.claude/projects/`.
Monitor disk usage and consider periodic cleanup.

**Goose**: Sessions are stored in a SQLite database with WAL mode enabled. You
must preserve the `-wal` and `-shm` sidecar files alongside the main database,
or the session database may become corrupted.

**Amazon Q**: The Amazon Q Developer CLI has been rebranded to Kiro CLI. The
existing module pins a specific CLI version. An authentication tarball is stored
alongside session data; if it is lost, the agent must re-authenticate.

**Gemini**: Session data can reach 400 MB for long-running tasks. You can set
the `general.sessionRetention` configuration value to control how long sessions
are retained.

**Sourcegraph Amp**: Conversation threads are stored server-side on
Sourcegraph servers, so only local configuration in `~/.config/amp/` needs
persistence. The workspace must have network connectivity to Sourcegraph for
resume to work.

**Auggie**: May require connectivity to the Augment cloud backend for session
resume. Behavior in fully headless or network-restricted environments is not
fully verified.

**Aider**: The `--restore-chat-history` flag performs a lossy reconstruction
from a Markdown log file, but the agent loses full conversation context on each
restart and does not support MCP for status reporting. When
`enable_state_persistence` is enabled in the module, the Coder UI preserves chat
history across pause and resume, but Aider itself starts each session fresh with no
memory of previous conversations.

## Next steps

- [Task lifecycle](./tasks-lifecycle.md) for how pause and resume work and
  what your template needs.
- [Set up Coder Tasks](./tasks.md) in your template.
- [Build a custom agent](./custom-agents.md) with MCP support.
