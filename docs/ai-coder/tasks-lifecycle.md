# Task lifecycle

> [!WARNING]
> Starting June 2, 2026, Coder Tasks will move to a 12-month Extended Support Release (ESR) for Premium customers.
>
> Tasks will be removed from new Coder releases beginning with v2.37 (September 1, 2026) and will only be available via the ESR during the support period.
>
> We recommend transitioning to [Coder Agents](./agents/index.md), the long-term replacement.

Tasks can pause when idle and resume when you interact with them again.
Pausing frees compute resources while preserving conversation context, so
the agent can pick up where it left off. This page covers how pause and
resume work, what gets preserved, and what your template needs.

> [!NOTE]
> Task pause and resume is in beta. Some details may change in future releases.

## How tasks pause

Tasks pause in two ways:

- **Auto-pause**: The workspace idle timeout expires. Tasks use the
  template's existing `default_ttl` and `activity_bump` settings, the same
  ones that control regular workspace auto-stop. When a task auto-pauses,
  the build reason is recorded as "idle timeout" and a notification is sent
  to the task owner.
- **Manual pause**: You can pause a task through the CLI with
  `coder task pause`, the API, or the pause button in the Tasks UI.

When a task pauses, the workspace stops. Compute resources are freed and
persistent storage remains intact. Stopping a task workspace manually (via
the workspace UI or `coder stop`) triggers the same pause behavior,
including log snapshot capture and state persistence. Similarly, starting
the workspace (`coder start`) resumes the task.

### Activity detection for tasks

AI agent activity extends the workspace deadline just like SSH or IDE
connections do. When an agent reports "working" status through Coder Tasks,
the workspace deadline is bumped by the template's `activity_bump` duration.
This prevents auto-pause while the agent is actively working.

See [Workspace scheduling](../user-guides/workspace-scheduling.md) for the
full list of activity types.

## What gets preserved

Three things survive a pause:

1. **Log snapshot**: Up to 30 of the last messages from the conversation
   are captured during shutdown and stored server-side. While paused,
   `coder task logs` and the Tasks UI show this snapshot so you can see
   what the agent was working on.

1. **AgentAPI state**: When state persistence is enabled, the full
   conversation history is saved to a file on persistent storage. After
   resume, the Tasks UI shows the complete chat history.

1. **AI agent session**: Agents that support session persistence (such as
   Claude Code via `~/.claude/`) retain their own context on persistent
   storage. On resume, the agent picks up where it left off with full
   memory of the previous conversation.

> [!NOTE]
> Log snapshots and AgentAPI state persistence are best-effort. If the
> shutdown script is interrupted or times out, the workspace still stops
> normally, but the snapshot may not be captured and chat history may be
> empty after resume.

If `enable_state_persistence` is true but the AI agent does not support
session resume, the UI shows previous messages but the agent starts fresh
with no memory of the conversation. This is expected behavior. See
[Agent compatibility](./agent-compatibility.md) for which agents support
full session resume.

## Resuming a task

You can resume a paused task in several ways:

- **CLI**: `coder task resume <task>`
- **UI**: Click the **Resume** button on the task page or in the tasks list

Resume starts the workspace, runs startup scripts, starts AgentAPI (which
loads its state file if state persistence is enabled), and starts the AI
agent (which resumes its session if supported).

> [!NOTE]
> Resume requires a full workspace build, which can take several minutes
> depending on your template.

## Requirements

### Persistent storage

Templates must have persistent storage (Docker volume, Kubernetes PVC, or
similar) that survives workspace stop and start cycles. Without it, the AI
agent's session files and the AgentAPI state file are lost on stop.

See
[Resource persistence](../admin/templates/extending-templates/resource-persistence.md)
for configuration patterns.

### Compatible module version

AI agent registry modules handle shutdown scripts and state persistence
through the agentapi base module. To enable pause and resume, use a module
version that includes this support.

For Claude Code, update the module version in your template:

```hcl
module "claude-code" {
  source   = "registry.coder.com/coder/claude-code/coder"
  version  = ">= 4.8.0" # Minimum version with pause/resume support
  agent_id = coder_agent.main.id
  # ...
}
```

Versions 4.8.0 and above set `enable_state_persistence = true`, which
configures the shutdown script and state file automatically.

See [Agent compatibility](./agent-compatibility.md) for the minimum module
version per agent.

#### The `enable_state_persistence` variable

The `enable_state_persistence` variable controls whether AgentAPI saves and
restores conversation history across pause and resume cycles. It defaults to
`false` in the agentapi base module. Agent modules that support session
persistence, like `claude-code`, override this to `true` in their module
definition.

When `enable_state_persistence` is `false`, the shutdown script still runs to
capture log snapshots, but skips saving AgentAPI state. On resume, chat
history is not restored.

If you are building a [custom agent](./custom-agents.md#pause-and-resume),
set this variable on the agentapi module directly.

### Graceful shutdown timeout

> [!WARNING]
> Without this configuration, log snapshots and state persistence may
> silently fail. The container runtime can terminate the container before
> the shutdown script finishes.

The shutdown script runs inside the workspace container. The container
runtime controls how long the process has to shut down before it is
force-terminated. The defaults are often too short:

- **Docker**: 10 seconds
- **Kubernetes**: 30 seconds

The grace period covers not just this shutdown script but also the workspace
agent's own graceful shutdown and any other modules that run shutdown
scripts. Set at least **1 minute** as a baseline. **5 minutes** is
recommended to account for slow disks, multiple shutdown scripts, and other
modules performing cleanup.

**Docker**: Add to your `docker_container` resource:

```hcl
resource "docker_container" "workspace" {
  # Both attributes are needed for graceful shutdown.
  destroy_grace_seconds = 300 # 5 minutes
  stop_timeout          = 300
  stop_signal           = "SIGINT"
  # ...
}
```

**Kubernetes**: Add to your `kubernetes_pod` resource:

```hcl
resource "kubernetes_pod" "main" {
  timeouts {
    delete = "6m" # Must exceed the grace period below.
  }
  spec {
    termination_grace_period_seconds = 300 # 5 minutes
  }
}
```

If the container is terminated before the shutdown script finishes, the workspace
still stops normally but log snapshots may be missing and chat history may
not be restored after resume.

## Next steps

- [Agent compatibility](./agent-compatibility.md) for session persistence
  support and minimum module versions.
- [Resource persistence](../admin/templates/extending-templates/resource-persistence.md)
  for configuring persistent storage in templates.
- [Workspace scheduling](../user-guides/workspace-scheduling.md) for how
  auto-stop and activity detection work.
