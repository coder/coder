# Process priority management

Coder's agent can automatically lower the scheduling priority
and raise the OOM (out-of-memory) kill score of user processes
so the agent itself stays alive under resource pressure. This
feature is Linux-only and available in the OSS edition.

When enabled, every command the agent spawns — SSH sessions,
reconnecting PTY terminals, and other child processes — is
wrapped with `coder agent-exec`, which adjusts nice and OOM
scores before exec'ing the target command.

## Prerequisites

- **Linux** — The feature is ignored on other operating systems.
- **`CAP_SYS_NICE`** — Required if the agent needs to lower
  the nice value below its current value, or adjust OOM scores
  in certain configurations. In Docker, add it with:

  ```hcl
  capabilities {
    add = ["CAP_SYS_NICE"]
  }
  ```

> [!NOTE]
> When Linux capabilities like `cap_net_admin` are set on
> the agent binary, the kernel disables `PR_SET_DUMPABLE`,
> which prevents adjusting `oom_score_adj`. The agent handles
> this automatically by dropping effective capabilities and
> re-enabling `PR_SET_DUMPABLE` before writing the OOM score,
> then disabling it again for safety.

## Environment variables

Configure the feature with environment variables on the
`coder_agent` resource. You can set these in the `env` block
of your template.

| Variable                | Required | Default                       | Description                                                                              |
|-------------------------|----------|-------------------------------|------------------------------------------------------------------------------------------|
| `CODER_PROC_PRIO_MGMT`  | Yes      | —                             | Set to any non-empty value (e.g., `1`) to enable the feature.                            |
| `CODER_PROC_OOM_SCORE`  | No       | Computed from agent's score   | Explicit `oom_score_adj` value for child processes. Range: `-1000` to `1000`.            |
| `CODER_PROC_NICE_SCORE` | No       | Agent nice + 5 (capped at 19) | Explicit nice value for child processes. Range: `-20` to `19` (higher = lower priority). |

### OOM score defaults

If you do not set `CODER_PROC_OOM_SCORE`, the agent computes a
value based on its own `oom_score_adj`:

| Agent's `oom_score_adj` | Child score | Rationale                                      |
|-------------------------|-------------|------------------------------------------------|
| Negative (< 0)          | `0`         | Children are treated as normal processes.      |
| >= 998                  | `1000`      | Children get the maximum score (killed first). |
| Any other value         | `998`       | Children get a near-maximum score.             |

The goal is for the kernel's OOM killer to target child
processes before the agent, keeping remote connectivity alive
even when a workspace runs out of memory.

### Nice score defaults

If you do not set `CODER_PROC_NICE_SCORE`, the agent sets
children to its own nice value plus 5, capped at 19. This
gives the agent more CPU scheduling priority than user
workloads.

## How it works

When `CODER_PROC_PRIO_MGMT` is set:

1. The agent resolves its own binary path at startup.
1. Every spawned command is wrapped as:

   ```sh
   coder agent-exec [--coder-oom=N] [--coder-nice=N] -- <original command>
   ```

1. The `agent-exec` subcommand then:
   1. Locks the OS thread (nice is per-thread on Linux).
   1. Drops effective capabilities for security.
   1. Sets `PR_SET_DUMPABLE` to allow OOM score adjustment.
   1. Writes the OOM score to `/proc/self/oom_score_adj`.
   1. Resets `PR_SET_DUMPABLE` to `0`.
   1. Calls `setpriority()` to set the nice value.
   1. Strips all `CODER_PROC_*` environment variables from
      the child environment.
   1. Calls `syscall.Exec()` to replace itself with the
      target command.

> [!NOTE]
> Errors during priority adjustment are logged to stderr but
> do **not** prevent the command from running. The user's
> session still starts normally.

## Example

Add the following `env` and `capabilities` blocks to a Docker
or Kubernetes template's `coder_agent` or container resource:

```hcl
resource "docker_container" "workspace" {
  # ... other configuration

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_PROC_PRIO_MGMT=1",
    "CODER_PROC_OOM_SCORE=10",
    "CODER_PROC_NICE_SCORE=1",
  ]

  capabilities {
    add = ["CAP_NET_ADMIN", "CAP_SYS_NICE"]
  }
}
```

- `CODER_PROC_OOM_SCORE=10` gives child processes a slightly
  elevated OOM score while keeping them well below the maximum.
- `CODER_PROC_NICE_SCORE=1` gives children a mildly lower CPU
  priority than the agent.
- `CAP_SYS_NICE` allows the agent to set nice and OOM values.
- `CAP_NET_ADMIN` is not required for process priority but is
  commonly included for improved networking performance.

## Troubleshooting

### OOM score adjustment fails silently

If you see `error adjusting oom score` in stderr but the
process still starts, the agent likely lacks permission to
write to `/proc/self/oom_score_adj`. Add `CAP_SYS_NICE` to
the container capabilities.

### Nice value is not applied

Nice values can only be increased (lowered in priority)
without `CAP_SYS_NICE`. If your template sets a
`CODER_PROC_NICE_SCORE` lower than the agent's current nice
value, you need the capability.

### Environment variables leak to nested Coder agents

The agent strips all `CODER_PROC_*` variables from child
environments automatically. This prevents interference in
"Coder on Coder" development scenarios where a workspace
runs another Coder agent.

### Feature has no effect on macOS or Windows

Process priority management is Linux-only. Setting
`CODER_PROC_PRIO_MGMT` on other operating systems is safe
but has no effect.
