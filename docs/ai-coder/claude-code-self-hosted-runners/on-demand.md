# On-demand runners: orchestrator + spawn hook

Run Claude Code self-hosted runners **on demand** instead of
maintaining a fixed fleet. An orchestrator process polls Anthropic for
pending spawn requests and invokes a `spawn-runner` hook that creates
a Coder workspace per session. Each workspace receives a single-use
work order (not the long-lived pool secret), serves exactly one
session, then exits.

> [!NOTE]
> This recipe has been prototyped and proven end-to-end. It is an
> alternative to the fixed-fleet prebuild model described in
> [System identity](./system-identity.md). Choose the model that fits
> your scaling needs.

## How it differs from the fixed fleet

| Aspect                  | Fixed fleet (prebuilds)                                    | On-demand (orchestrator)                     |
|-------------------------|------------------------------------------------------------|----------------------------------------------|
| Pool sizing             | Static `instances = N`                                     | Elastic, one workspace per session           |
| Credential in workspace | Long-lived pool secret                                     | Single-use work order JWT                    |
| Cold start              | Near-zero (prebuild is already running)                    | Workspace creation time (seconds to minutes) |
| Pool secret exposure    | Inside every runner workspace                              | Only on the orchestrator host                |
| Lifecycle               | Runner drains, workspace self-deletes, reconciler rebuilds | Runner serves one session, workspace exits   |
| Coder Premium required  | Yes (prebuilds)                                            | No (plain workspace creation)                |

## Architecture

Three components work together:

1. **Orchestrator host.** A machine (VM, container, or bare-metal)
   outside your runner workspaces that runs
   `claude self-hosted-runner orchestrator`. It holds the pool secret,
   polls Anthropic for spawn hints, and invokes the `spawn-runner`
   hook once per session.
2. **`spawn-runner` hook.** A shell script in the orchestrator's hooks
   directory. It calls the Coder REST API to create a workspace,
   passing the single-use work order as an ephemeral parameter.
3. **On-demand runner template.** A Coder template whose workspaces
   run `claude self-hosted-runner --capacity 1` with the work order
   from the ephemeral parameter. The runner registers, serves the
   session, and exits.

```text
claude.ai/code
    │
    ▼
Anthropic pool scheduler
    │  spawn hint
    ▼
Orchestrator host (claude self-hosted-runner orchestrator)
    │  invokes spawn-runner hook
    ▼
spawn-runner hook
    │  POST /api/v2/.../workspaces (work order as parameter)
    ▼
Coder workspace (on-demand runner template)
    │  claude self-hosted-runner --capacity 1
    ▼
Session served, runner exits
```

## Prerequisites

Everything from the [System identity prerequisites](./system-identity.md#prerequisites),
plus:

- **A host to run the orchestrator.** This does not need to be a Coder
  workspace. It needs outbound HTTPS to `api.anthropic.com`, the
  `claude` binary installed, and network access to your Coder
  deployment's API.
- **The `claude` binary on the orchestrator host.** Same tarball as
  used in the runner image.
- **A Coder API token** for the service account that creates runner
  workspaces (`svc-claude-delete` or equivalent). The `spawn-runner`
  hook uses this to call the workspace-create API.
- **The on-demand runner template** published to Coder (see below).

## Step 1: Create the on-demand runner template

The template is similar to the fixed-fleet template but uses an
**ephemeral `work_order` parameter** instead of a sensitive
`pool_secret` variable. The orchestrator passes a single-use work
order JWT when it creates each workspace.

```hcl
# On-demand runner workspace template.
#
# Each workspace is spawned by the orchestrator's spawn-runner hook with
# a single-use work order. The workspace runs `claude self-hosted-runner`
# for exactly one session, then self-destructs.

terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

provider "docker" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# The work order JWT, passed by the spawn-runner hook as an ephemeral
# parameter. This is a single-use credential that registers exactly one
# runner for exactly one session. The runner reads it from
# SELF_HOSTED_RUNNER_POOL_SECRET (same env var as the pool secret, but
# the orchestrator substitutes a work order instead).
data "coder_parameter" "work_order" {
  name         = "work_order"
  display_name = "Work order"
  description  = "Single-use work order JWT from the orchestrator. Do not set manually."
  type         = "string"
  ephemeral    = true
  mutable      = true
  default      = ""
}

resource "coder_agent" "main" {
  arch                    = data.coder_provisioner.me.arch
  os                      = "linux"
  dir                     = "/home/coder"
  startup_script_behavior = "non-blocking"

  env = {
    # The runner reads the work order from this env var.
    SELF_HOSTED_RUNNER_POOL_SECRET = data.coder_parameter.work_order.value
    # Workspace ID for self-deletion on drain.
    CODER_WORKSPACE_ID = data.coder_workspace.me.id
  }
}

resource "coder_script" "runner" {
  agent_id     = coder_agent.main.id
  display_name = "Claude Code runner"
  icon         = "/emojis/1f916.png"
  run_on_start = true
  script       = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail

    if [ -z "$${SELF_HOSTED_RUNNER_POOL_SECRET:-}" ]; then
      echo "No work order set. This workspace was created manually, not by the orchestrator."
      echo "Idling. Delete this workspace manually or set the work_order parameter."
      exit 0
    fi

    echo "Starting on-demand runner with work order..."

    # Run the runner. It will register with the work order, serve the
    # session, and exit when the session completes.
    claude self-hosted-runner --capacity 1
    status=$?

    echo "Runner exited with status $status"

    # Self-destruct: the workspace's job is done.
    if command -v coder >/dev/null 2>&1 && [ -n "$${CODER_WORKSPACE_ID:-}" ]; then
      echo "Self-deleting workspace..."
      coder delete --yes "$(coder show --json "$CODER_WORKSPACE_ID" 2>/dev/null \
        | jq -r '.name' 2>/dev/null || echo "$CODER_WORKSPACE_ID")" 2>&1 || true
    fi

    exit $status
  EOT
}

# Use the same runner image as the fixed-fleet template.
resource "docker_image" "runner" {
  name = "coder-claude-on-demand-runner:${data.coder_workspace.me.id}"
  build {
    context = "${path.module}/build"
  }
  triggers = {
    build_sha1 = sha1(join("", [for f in fileset(path.module, "build/*") : filesha1("${path.module}/${f}")]))
  }
  keep_locally = true
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = docker_image.runner.name
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  user     = "coder"

  entrypoint = [
    "sh", "-c",
    replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal"),
  ]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
  ]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
    read_only      = false
  }
}
```

Key differences from the fixed-fleet template:

- **Ephemeral `work_order` parameter** instead of a sensitive
  `pool_secret` variable. The work order is a single-use JWT scoped
  to one session.
- **`--capacity 1`** because each workspace serves exactly one session.
- **No prebuilds preset.** Workspaces are created on demand by the
  orchestrator hook, not maintained as a warm pool.
- **Self-deletion** after the runner exits. The workspace's job is
  done once the session completes.

The workspace image is the same one used for the fixed-fleet template.
See [Step 2 in System identity](./system-identity.md#step-2-bake-the-runner-into-a-workspace-image)
for the Dockerfile.

Push the template:

```bash
coder templates push claude-on-demand-runner --yes
```

## Step 2: Write the spawn-runner hook

The orchestrator invokes the `spawn-runner` hook once per spawn hint.
The hook must be idempotent on `CLAUDE_RUNNER_ORDER_ID` (re-delivery
must not double-spawn), and must return within the hook timeout
(default 60 seconds). It should not wait for the workspace to be
ready; just submit the create and return.

Exit codes:

- `0` = success (workspace created or already exists)
- `1` = retryable failure
- `>= 2` = non-retryable (permanent) failure

Create a hooks directory on the orchestrator host and add the
`spawn-runner` script:

```bash
#!/usr/bin/env bash
# spawn-runner hook for Coder workspaces.
# Invoked by `claude self-hosted-runner orchestrator` once per spawn hint.
#
# Contract (from Anthropic's runner guide):
#   - Idempotent on CLAUDE_RUNNER_ORDER_ID (re-delivery must not double-spawn)
#   - No provisioner-side retry (one ORDER_ID = at most one workspace)
#   - Exit 0 = success, 1 = retryable, >=2 = non-retryable
#   - Must return within --hook-timeout (default 60s); do NOT wait for
#     the workspace to be ready, just submit the create and return.
set -eu

: "${CLAUDE_RUNNER_WORK_ORDER_FILE:?required (set by orchestrator)}"
: "${CLAUDE_RUNNER_ORDER_ID:?required (set by orchestrator)}"
: "${CODER_TEMPLATE:?required}"
: "${CODER_URL:?required}"
: "${CODER_SESSION_TOKEN:?required}"

# Workspace name derived from order ID (already DNS-label safe, <= 63 chars).
NAME="run-$(printf '%s' "$CLAUDE_RUNNER_ORDER_ID" | cut -c1-28)"

log() { printf '[spawn-runner] %s\n' "$*" >&2; }

# Read the work order.
WORK_ORDER="$(cat "$CLAUDE_RUNNER_WORK_ORDER_FILE")"

# Idempotency check: if the workspace already exists, exit 0.
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
  "${CODER_URL}/api/v2/users/svc-claude-delete/workspace/${NAME}")

if [ "$HTTP_CODE" = "200" ]; then
  log "workspace $NAME already exists (re-delivery), exiting 0"
  exit 0
fi

log "creating workspace $NAME for session ${CLAUDE_RUNNER_SESSION_ID:-unknown}"
log "account: ${CLAUDE_RUNNER_ACCOUNT_ID:-unknown}"
log "repo: ${CLAUDE_RUNNER_PRIMARY_REPO_URL:-none}"

# Get the template ID.
TEMPLATE_ID=$(curl -s \
  -H "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
  "${CODER_URL}/api/v2/organizations/default/templates/${CODER_TEMPLATE}" \
  | jq -r '.id')

if [ -z "$TEMPLATE_ID" ] || [ "$TEMPLATE_ID" = "null" ]; then
  log "ERROR: could not find template $CODER_TEMPLATE"
  exit 2  # non-retryable
fi

# Create the workspace via the REST API with the work order as an
# ephemeral parameter.
RESPONSE=$(curl -s -w "\n%{http_code}" \
  -X POST \
  -H "Coder-Session-Token: ${CODER_SESSION_TOKEN}" \
  -H "Content-Type: application/json" \
  "${CODER_URL}/api/v2/organizations/default/members/svc-claude-delete/workspaces" \
  -d "$(jq -n \
    --arg name "$NAME" \
    --arg template_id "$TEMPLATE_ID" \
    --arg work_order "$WORK_ORDER" \
    '{
      name: $name,
      template_id: $template_id,
      rich_parameter_values: [
        {name: "work_order", value: $work_order}
      ]
    }')")

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
  log "workspace $NAME created (HTTP $HTTP_CODE)"
  exit 0
elif [ "$HTTP_CODE" = "409" ]; then
  log "workspace $NAME already exists (HTTP 409, idempotent)"
  exit 0
else
  log "ERROR: workspace create failed (HTTP $HTTP_CODE): $BODY"
  exit 1  # retryable
fi
```

> [!NOTE]
> The hook uses the Coder REST API directly
> (`POST /api/v2/organizations/default/members/{user}/workspaces`)
> rather than `coder create` because the CLI does not yet support
> `--ephemeral-parameter` for passing the work order at create time.

Make the hook executable:

```bash
chmod +x /path/to/hooks/spawn-runner
```

Set the environment variables the hook expects. Export these on the
orchestrator host before starting the orchestrator:

```bash
export CODER_URL="https://coder.example.com"
export CODER_SESSION_TOKEN="<svc-claude-delete API token>"
export CODER_TEMPLATE="claude-on-demand-runner"
```

## Step 3: Start the orchestrator

Run the orchestrator on the host machine. It needs the pool secret
file and the hooks directory:

```bash
claude self-hosted-runner orchestrator \
  --pool-secret-file /path/to/pool-secret \
  --hooks-dir /path/to/hooks \
  --health-port 8081 \
  --expected-spawn-seconds 180
```

Flags:

- `--pool-secret-file`: path to the file containing the pool secret.
- `--hooks-dir`: directory containing the `spawn-runner` script.
- `--health-port`: port for the orchestrator's health endpoint.
- `--expected-spawn-seconds`: how long the orchestrator waits for a
  spawned runner to register before retrying. Set this to account for
  workspace build time.

The orchestrator polls Anthropic for pending spawn hints, invokes
`spawn-runner` for each one, and handles retries, idempotency, and
circuit-breaking.

## Step 4: Verify

1. Confirm the orchestrator is running and healthy:

   ```bash
   curl -s http://localhost:8081/healthz
   ```

2. Start a Claude Code session at `claude.ai/code` and select the
   pool that targets the orchestrator.

3. Watch the orchestrator logs. You should see:
   - A spawn hint received from Anthropic.
   - The `spawn-runner` hook invoked.
   - A workspace created via the Coder API (HTTP 201).

4. In Coder's workspace list, a new workspace named `run-<order-id>`
   should appear under the service account, transition through
   building to running, and show the runner process starting.

5. The runner registers with the work order, picks up the session,
   and the developer sees the session in the Anthropic UI.

6. When the session completes, the runner exits and the workspace
   self-deletes.

## Operate

### Logs

- **Orchestrator logs**: stderr on the orchestrator host. Includes
  spawn hint receipts, hook invocations, and retry/circuit-breaker
  state.
- **Runner logs**: inside each workspace at `~/.claude/runner.log`
  and in the Coder workspace agent logs.
- **Hook logs**: the `spawn-runner` hook writes to stderr, which the
  orchestrator captures.

### Credential isolation

The pool secret never enters a runner workspace. It stays on the
orchestrator host. Each workspace receives a single-use work order
JWT that is valid for exactly one runner registration. A compromised
workspace cannot impersonate the pool or spawn additional runners.

### Cleanup

Workspaces self-delete after the runner exits. If self-deletion
fails (for example, the Coder API is unreachable), set a workspace
TTL on the template as a backstop so stale workspaces are reclaimed.

### Scaling

The orchestrator creates one workspace per session. There is no
static pool to size. The upper bound on concurrency is whatever your
infrastructure can handle in terms of simultaneous workspace builds
and running containers.

To limit concurrency, configure resource quotas on the Coder
deployment or the underlying infrastructure (Docker, Kubernetes, or
cloud provider limits).

## Common pitfalls

| Symptom                                         | Cause and fix                                                                                                                                                                                                     |
|-------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Hook exits 2, orchestrator does not retry       | The template name in `CODER_TEMPLATE` does not match. Confirm the template exists with `coder templates list`.                                                                                                    |
| Workspace created but runner never registers    | The work order did not reach the workspace. Check that `rich_parameter_values` in the hook payload includes `{name: "work_order", value: ...}` and that the template's ephemeral parameter is named `work_order`. |
| `EACCES: permission denied, mkdir '/workspace'` | Same image fix as the fixed-fleet template. See [System identity common pitfalls](./system-identity.md#common-pitfalls).                                                                                          |
| Orchestrator retries the same order repeatedly  | The spawned workspace is failing before the runner registers. Check workspace build logs in Coder. Increase `--expected-spawn-seconds` if the build is slow but succeeding.                                       |
| Stale workspaces accumulate                     | Self-deletion failed. Set a workspace TTL on the template as a backstop. Check that `coder` CLI is available in the image and `CODER_WORKSPACE_ID` is set.                                                        |

## Where to next

- [System identity](./system-identity.md): the fixed-fleet prebuild
  model, for when you want zero cold-start latency.
- [User identity](./user-identity.md): per-developer attribution.
  On the Coder + Anthropic roadmap; not yet available.
- [Implementation notes](./plan.md): the staged plan and open
  questions.
