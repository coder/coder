# Improving Agent Resiliency

Coder's agent can automatically lower the scheduling priority
and raise the OOM (out-of-memory) kill score of user processes
so the agent itself stays alive under resource pressure.

## Prerequisites

- **Linux** — The feature is ignored on other operating systems.
- **`CAP_SYS_NICE`** — Required if the agent needs to lower
  the nice value below its current value. In Kubernetes, add
  it to the container's security context:

  ```hcl
  container {
    security_context {
      capabilities {
        add = ["CAP_SYS_NICE"]
      }
    }
  }
  ```

## Environment variables

Configure the feature with environment variables in the
environment that launches the agent binary. These must be set
on the workspace container or host, not in the `coder_agent`
resource's `env` block — the agent reads them from its own
process environment at startup.

| Variable                | Required | Default                       | Description                                                                                                                                                                                   |
|-------------------------|----------|-------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `CODER_PROC_PRIO_MGMT`  | Yes      | —                             | Set to enable the feature. The agent checks whether the variable is present, not its value — even an empty string enables it. Use `1` by convention. To disable, unset the variable entirely. |
| `CODER_PROC_OOM_SCORE`  | No       | Computed from agent's score   | Explicit `oom_score_adj` value for child processes. Range: `-1000` to `1000`.                                                                                                                 |
| `CODER_PROC_NICE_SCORE` | No       | Agent nice + 5 (capped at 19) | Explicit nice value for child processes. Range: `-20` to `19` (higher = lower priority).                                                                                                      |

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

## Example

The following Kubernetes template snippet enables process
priority management on the workspace container:

```hcl
resource "kubernetes_deployment" "workspace" {
  # ... other configuration

  spec {
    template {
      spec {
        container {
          name  = "dev"
          image = "codercom/enterprise-base:ubuntu"

          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          env {
            name  = "CODER_PROC_PRIO_MGMT"
            value = "1"
          }
          env {
            name  = "CODER_PROC_OOM_SCORE"
            value = "10"
          }
          env {
            name  = "CODER_PROC_NICE_SCORE"
            value = "1"
          }

          security_context {
            capabilities {
              add = ["CAP_SYS_NICE"]
            }
          }
        }
      }
    }
  }
}
```

- `CODER_PROC_OOM_SCORE=10` gives child processes a slightly
  elevated OOM score while keeping them well below the maximum.
- `CODER_PROC_NICE_SCORE=1` gives children a mildly lower CPU
  priority than the agent.
- `CAP_SYS_NICE` allows the agent to set nice values.

## Troubleshooting

### OOM score adjustment fails

If you see `failed to adjust oom score` in stderr but the
process still starts, the agent likely lacks permission to
write to `/proc/self/oom_score_adj`. Ensure the process is
dumpable — this is handled automatically by the agent, but
can fail if the container runtime restricts `prctl` calls.

### Nice value is not applied

If you see `failed to adjust niceness` in stderr, nice values
can only be increased (lowered in priority) without
`CAP_SYS_NICE`. If your template sets a `CODER_PROC_NICE_SCORE`
lower than the agent's current nice value, add the capability
to the container's security context.

### Environment variables leak to nested Coder agents

The agent strips all `CODER_PROC_*` variables from child
environments automatically. This prevents interference in
"Coder on Coder" development scenarios where a workspace
runs another Coder agent.

### Verifying the feature is enabled

The agent logs whether process priority management is active
at startup. Look for these lines in the agent log:

```text
"process priority management enabled"
"process priority management not enabled (linux-only)"
```

The log entry includes the `CODER_PROC_PRIO_MGMT` value and
the operating system. Check the agent log file at
`<log-dir>/coder-agent.log` or stderr output.

### Feature has no effect on macOS or Windows

Process priority management is Linux-only. Setting
`CODER_PROC_PRIO_MGMT` on other operating systems is safe
but has no effect.
