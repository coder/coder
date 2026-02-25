# AI Session Capture

`coder-capture` is a Terraform module that installs git hooks and agent-specific
hooks into Coder workspaces to capture AI coding session activity. It links git
events — commits, pushes, and branch changes — to the AI tool that generated
them, giving template administrators and platform teams visibility into how AI
coding assistants are being used across workspaces.

Captured events are sent to the Coder control plane API and surfaced in the
admin dashboard. Each event carries a session identifier that ties it to a
specific AI coding session, so administrators can answer questions like: which
AI tool generated this commit, how many AI-assisted commits happened this week,
or which workspaces are most actively using AI coding assistants.

The module works by writing standard git hooks (`post-commit`, `commit-msg`,
`pre-push`) into each workspace's git repositories, along with tool-specific
hooks for Claude Code and Gemini CLI that fire at session start and end. When a
hook fires, it reads the current AI session ID from the environment and ships
the event to the agent's local API, which forwards it to the Coder control
plane.

## Quick Start

Add the module to your template and point it at your workspace agent:

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
}

module "coder-capture" {
  source   = "./modules/coder-capture"
  agent_id = coder_agent.dev.id
}
```

The module installs itself during the agent's startup sequence. No additional
agent configuration is required.

## Configuration Options

| Variable       | Type   | Default         | Description                                                                                          |
|----------------|--------|-----------------|------------------------------------------------------------------------------------------------------|
| `agent_id`     | string | *(required)*    | The ID of the `coder_agent` resource to attach hooks to.                                             |
| `no_trailer`   | bool   | `false`         | When `true`, disables appending AI session metadata to commit messages. Useful for strict commit-msg policies. |
| `log_dir`      | string | `/tmp/coder-capture` | Directory where hook execution logs are written. Must be writable by the workspace user.        |

Example with all options:

```hcl
module "coder-capture" {
  source     = "./modules/coder-capture"
  agent_id   = coder_agent.dev.id
  no_trailer = false
  log_dir    = "/home/coder/.coder-capture/logs"
}
```

## How It Works

### Git Hooks

`coder-capture` installs three git hooks globally in each workspace via
`git config --global core.hooksPath`. This means hooks apply to every
repository the user works in without per-repo setup.

| Hook          | Trigger                              | Captured Data                                      |
|---------------|--------------------------------------|----------------------------------------------------|
| `commit-msg`  | Before a commit message is saved     | Injects session ID trailer (unless `no_trailer` is set) |
| `post-commit` | After a commit is created            | Commit hash, author, timestamp, session ID         |
| `pre-push`    | Before a push to a remote            | Remote URL, branch name, commit range, session ID  |

The hooks are shell scripts that call a small binary installed by the module.
If the binary is unavailable or the API call fails, the hook exits cleanly so
that git operations are never blocked.

### Session Detection

The module determines the current AI session ID through a priority chain:

1. **Claude Code hooks** — When Claude Code is active, it sets
   `CLAUDE_SESSION_ID` in the hook environment. The module installs a Claude
   Code `PreToolUse` hook that fires at the start of each tool use, and a
   `PostToolUse` hook that fires at the end, allowing precise session
   boundaries to be recorded.

2. **Gemini CLI hooks** — When Gemini CLI is active, it sets
   `GEMINI_SESSION_ID` via its hook mechanism. The module registers a Gemini
   hook script that fires on session start and end.

3. **Fallback UUID** — For tools that do not expose native hooks (Codex,
   Cursor, Aider, and others), the module generates a stable UUID derived from
   the workspace ID and the current process group. This UUID persists for the
   lifetime of the tool process, providing a best-effort session boundary.

If no AI tool is detected, events are still captured under an anonymous session
so that git activity is never silently dropped.

### Data Flow

```
Workspace (git hook fires)
        │
        ▼
coder-capture binary
        │  HTTP POST to agent local API
        ▼
Coder Agent (localhost)
        │  Forwards to control plane
        ▼
Coder Control Plane API
        │  Stored in database
        ▼
Admin Dashboard (/aibridge/*)
```

The agent acts as a local proxy, buffering events when the control plane is
temporarily unreachable and retrying with exponential backoff. Events are
deduplicated by commit hash so that retries do not produce duplicate records.

## Tool Coverage

| Tool         | Session Detection       | Commit Capture | Push Capture |
|--------------|-------------------------|----------------|--------------|
| Claude Code  | Native hooks (`CLAUDE_SESSION_ID`) | ✅ git hooks | ✅ git hooks |
| Gemini CLI   | Native hooks (`GEMINI_SESSION_ID`) | ✅ git hooks | ✅ git hooks |
| Codex        | Fallback UUID           | ✅ git hooks | ✅ git hooks |
| Cursor       | Fallback UUID           | ✅ git hooks | ✅ git hooks |
| Aider        | Fallback UUID           | ✅ git hooks | ✅ git hooks |
| Manual (no AI tool) | Anonymous session | ✅ git hooks | ✅ git hooks |

Tools with native hook support provide accurate session start and end times.
Tools using the fallback UUID provide commit and push events but may group
unrelated commits under the same session ID if multiple tool invocations share
a process group.

## AI Bridge Integration

`coder-capture` captures git events and session boundaries on its own. When
combined with the [AI Bridge](./integrations/ai-bridge.md) feature, the two
systems share a session ID so that LLM conversation logs are linked to the git
events they produced.

### Without AI Bridge

`coder-capture` records:

- Session start and end times (for tools with native hooks)
- Every commit and its metadata
- Every push and the commit range it contained
- The session ID associated with each event

This gives administrators git-level visibility into AI activity without
capturing the content of conversations.

### With AI Bridge

When AI Bridge is also enabled, the session ID written into git event records
matches the conversation ID stored by AI Bridge. This allows the dashboard to
display a complete picture: the user's conversation with the AI tool alongside
the commits that resulted from it.

To enable both, set the AI Bridge environment variables in your template before
starting the agent:

```hcl
resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"

  env = {
    # AI Bridge endpoint — points at the local AI Bridge proxy.
    CODER_AI_BRIDGE_URL = "http://localhost:4900"

    # Share the session ID across both systems.
    CODER_CAPTURE_SESSION_SOURCE = "aibridge"
  }

  startup_script = <<-EOF
    # Start AI Bridge proxy first so coder-capture can reach it.
    coder-aibridge &
    sleep 1
  EOF
}

module "coder-capture" {
  source   = "./modules/coder-capture"
  agent_id = coder_agent.dev.id
}
```

With both systems running, each LLM conversation appears in the dashboard with
an expandable list of the commits generated during that conversation.

## Admin Dashboard

Captured events are visible in the Coder admin dashboard under the AI Bridge
section. The following views are available to users with the `Template
Administrator` role or above.

### Sessions View — `/aibridge/sessions`

Lists all AI coding sessions across all workspaces. Each row shows:

- Session ID and the AI tool that generated it
- Workspace and owner
- Session start and end time (where available)
- Number of commits and pushes made during the session

Click a session to see the individual events it contains.

### Git Events View — `/aibridge/git-events`

A chronological log of every commit and push event captured across all
workspaces. Filterable by workspace, user, AI tool, date range, and repository.
Useful for compliance audits or investigating unexpected commits.

### Dashboard View — `/aibridge/dashboard`

Aggregate metrics across your deployment:

- AI tool usage breakdown (commits by tool)
- Sessions per day/week/month
- Most active workspaces and users
- Commit volume trends over time

## Data Format

### Event Types

| Event Type      | When It Fires                                     |
|-----------------|---------------------------------------------------|
| `session_start` | When an AI tool's native hook fires at startup    |
| `commit`        | Immediately after a git commit is created         |
| `push`          | Just before a push is sent to the remote          |
| `session_end`   | When an AI tool's native hook fires at shutdown   |

### JSON Schema

All events share a common envelope. Here is an example `commit` event:

```json
{
  "event_type": "commit",
  "session_id": "claude-abc123def456",
  "workspace_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "7f3e9a1b-4c2d-4e8f-9a1b-3c2d4e8f9a1b",
  "timestamp": "2025-02-25T14:32:10Z",
  "tool": "claude-code",
  "payload": {
    "commit_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
    "author_email": "user@example.com",
    "author_name": "Jane Smith",
    "message": "feat: add user authentication\n\nAI-Session: claude-abc123def456",
    "repository": "git@github.com:example/myapp.git",
    "branch": "feature/auth"
  }
}
```

A `session_start` event looks like:

```json
{
  "event_type": "session_start",
  "session_id": "claude-abc123def456",
  "workspace_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "7f3e9a1b-4c2d-4e8f-9a1b-3c2d4e8f9a1b",
  "timestamp": "2025-02-25T14:30:00Z",
  "tool": "claude-code",
  "payload": {}
}
```

Events are delivered to `POST /api/v2/workspaceagents/{agent_id}/gitevents` and
stored in the control plane database. The endpoint accepts a batch of up to 100
events per request.

## Troubleshooting

### Hooks are not firing

**Symptom**: Commits and pushes appear in the Git Events view with a significant
delay or not at all.

**Check**:

```sh
# Verify the global hooks path is set correctly.
git config --global core.hooksPath

# Expected output: /home/coder/.coder-capture/hooks
# (or the equivalent path for your workspace user)
```

If the path is missing or points elsewhere, the module did not complete its
startup sequence. Check the agent startup logs:

```sh
cat /tmp/coder-capture/startup.log
```

If the module installed but the hooks directory is wrong, re-run the startup
script by rebuilding the workspace.

### API calls are failing

**Symptom**: Hook logs show `connection refused` or `unauthorized` errors.

```sh
# Check recent hook execution logs.
tail -n 50 /tmp/coder-capture/hook.log
```

Common causes:

- **Agent not yet ready**: The hook fired before the Coder Agent finished
  connecting to the control plane. The binary retries automatically; if the
  error is transient it will resolve on the next commit.
- **Wrong agent token**: The `CODER_AGENT_TOKEN` environment variable is not
  set or has expired. Confirm it is present in the hook environment:

  ```sh
  env | grep CODER_AGENT_TOKEN
  ```

- **Firewall blocking localhost**: Some workspace images apply restrictive
  `iptables` rules. Confirm the agent API is reachable:

  ```sh
  curl -s http://localhost:4/api/v2/buildinfo
  ```

### Session ID is not being detected

**Symptom**: Events appear in the dashboard but the `session_id` field is a
generic UUID rather than a tool-specific ID, even when using Claude Code or
Gemini CLI.

**Check** that the tool's hook mechanism is enabled:

- **Claude Code**: Hooks must be enabled in `~/.claude/settings.json`. Ensure
  `hooks.enabled` is `true` and that the hook scripts installed by
  `coder-capture` are listed under `PreToolUse` and `PostToolUse`.

  ```sh
  cat ~/.claude/settings.json | grep -A5 hooks
  ```

- **Gemini CLI**: Hook scripts must be present in `~/.gemini/hooks/`. Confirm:

  ```sh
  ls ~/.gemini/hooks/
  # Expected: session-start.sh  session-end.sh
  ```

If the scripts are missing, the module's startup script may not have run
during this workspace session. Restart the workspace to trigger the startup
script again.

### Commits from non-AI work are captured

This is expected behavior. `coder-capture` captures all git events in the
workspace, not just those made by AI tools. Events without a detected session
ID are filed under an anonymous session. Template administrators can filter
these out in the Git Events view using the **Tool: None** filter.
