# Personal Workers

Personal Workers is the per-developer path: each developer runs one
Coder workspace with the Cursor worker started under **their own**
personal Cursor API key. The workspace, the worker, the git push
credential, and the worker's Cursor identity are the developer
themselves. Per-user identity end to end.

If you're on a Cursor Team plan, or want per-user identity today
without a Cursor Enterprise contract, this is the recipe.

For an admin-operated central pool, see [Worker Pool](./system-identity.md).

> [!IMPORTANT]
> Personal Workers register as **personal machines** in Cursor, not as
> pool workers. Each developer triggers their sessions with `worker=`
> or `machine=` from Slack, GitHub, Linear, or the Cloud Agents
> dashboard. There is no shared inventory; Alice's workers don't serve
> Bob's sessions.

## When to use this

- **Cursor Team plan** (or any plan; Worker Pool requires Enterprise).
- **You want per-user identity today.** Workspace owner is the human,
  Coder external auth wires their git push token, audit log attributes
  to them, the worker registers in Cursor under their identity.
- **You want each developer to manage their own capacity.** No central
  pool sizing problem.

## Limitations

- **One workspace = one user = one repo.** Standard 1:1:1; same as
  Worker Pool, with personal ownership.
- **No shared pool, no warm-start across users.** Each developer pays
  the workspace cold-start cost on first use of the day. Coder workspace
  auto-start hides this for active workspaces; only the first session
  of the day is slow.
- **Each user owns their Cursor API key.** Generated at
  `cursor.com/dashboard/integrations`. Rotation is the developer's
  responsibility, not the platform team's.
- **Up to 10 personal workers per user, 50 per team.** Cursor's
  self-hosted ceiling. Rarely a constraint with one workspace per repo.

## Identity model

| Layer              | Identity                                                |
|--------------------|---------------------------------------------------------|
| Coder workspace    | Owned by the developer                                  |
| Git author         | The developer                                           |
| Git push           | Enabled via Coder external auth                         |
| Cursor worker      | Authenticated with the developer's personal API key     |
| Coder audit log    | Attributes to the developer                             |
| Per-session signal | `activeBcId` in Cursor's fleet API, same as Worker Pool |

## Prerequisites

- A Coder deployment, OSS or Premium.
- Each developer:
  - A Cursor account on Team plan or higher.
  - A personal API key created at `cursor.com/dashboard/integrations`.
- A workspace base image that can install `cursor-agent`. You can
  share one base image with Worker Pool if you run both.
- Outbound HTTPS access from the workspace to `api.cursor.com`.

## Step 1: Each developer creates a Cursor personal API key

Each developer follows this once:

1. Sign in to `cursor.com`.
2. Go to **Dashboard > Integrations**.
3. Create a new API key. Copy the value; it is shown once.

## Step 2: Bake the `cursor-agent` binary into a workspace image

Identical to the [Worker Pool image step](./system-identity.md#step-2-bake-the-cursor-agent-binary-into-a-workspace-image).
You can share one base image between both recipes; only the template
configuration differs.

## Step 3: Publish the Coder template

The template defines:

- A workspace that runs `cursor-agent worker start` from
  `coder_agent.startup_script` with the developer's personal API key.
- One **per-workspace parameter** (not a template variable) for the
  Cursor API key. Each developer pastes their own when they create the
  workspace.
- One parameter for the git repo URL.
- `coder_agent.metadata` blocks that surface worker process state.

The key difference from Worker Pool: the Cursor API key is a
**per-workspace parameter** so each developer's workspace runs under
their own identity, not a single sensitive template variable shared
fleet-wide.

```hcl
terraform {
  required_providers {
    coder  = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

data "coder_provisioner"     "me" {}
data "coder_workspace"       "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "cursor_api_key" {
  name         = "cursor_api_key"
  display_name = "Cursor personal API key"
  description  = "Generate at cursor.com/dashboard/integrations. Stored encrypted on the workspace."
  type         = "string"
  mutable      = true
}

data "coder_parameter" "git_repo_url" {
  name         = "git_repo_url"
  display_name = "Git Repository URL"
  description  = "Repository this worker serves."
  type         = "string"
  default      = "https://github.com/your-org/your-repo"
  mutable      = false
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
  dir  = "/home/coder"

  env = {
    CURSOR_API_KEY = data.coder_parameter.cursor_api_key.value
  }

  startup_script = <<-EOT
    set -eu
    export PATH="/usr/local/bin:$PATH"

    REPO_DIR="$HOME/workspace"
    REPO_URL="${data.coder_parameter.git_repo_url.value}"
    WORKER_LABEL="coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

    if [ ! -d "$REPO_DIR/.git" ]; then
      rm -rf "$REPO_DIR"
      git clone "$REPO_URL" "$REPO_DIR"
    else
      cd "$REPO_DIR"
      git remote set-url origin "$REPO_URL"
      git fetch --prune origin
      git reset --hard origin/HEAD
      git clean -fd
    fi

    cd "$REPO_DIR"

    # The workspace owner is the human, so git push works via Coder
    # external auth. No push block needed.

    git config --global user.name  "${coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)}"
    git config --global user.email "${data.coder_workspace_owner.me.email}"

    cursor-agent \
      --api-key "$CURSOR_API_KEY" \
      worker \
      --worker-dir         "$REPO_DIR" \
      --management-addr    ":8080" \
      --name               "$WORKER_LABEL" \
      start >> "$HOME/cursor-agent.log" 2>&1
  EOT

  metadata {
    display_name = "CPU"
    key          = "0_cpu"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM"
    key          = "1_ram"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Worker process"
    key          = "2_worker_process"
    interval     = 10
    timeout      = 2
    script       = <<-EOS
      if pgrep -f "cursor-agent worker" > /dev/null 2>&1; then echo running
      else echo stopped; fi
    EOS
  }

  metadata {
    display_name = "Ready (idle)"
    key          = "3_ready"
    interval     = 5
    timeout      = 3
    script       = <<-EOS
      if curl -fsS -o /dev/null http://127.0.0.1:8080/readyz; then echo idle
      else echo busy-or-starting; fi
    EOS
  }
}

resource "docker_volume" "home" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle { ignore_changes = all }
}

resource "docker_image" "worker" {
  name = "your-org/cursor-worker:latest"
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = docker_image.worker.name
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  user     = "coder"

  entrypoint = ["sh", "-c", coder_agent.main.init_script]

  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home.name
    read_only      = false
  }
}
```

## Step 4: Push the template

```bash
coder templates push cursor-personal-worker --yes
```

No template-level variables required; each developer provides their own
key when they create a workspace.

## Step 5: Each developer creates their workspace

```bash
coder create my-cursor-worker \
  --template cursor-personal-worker \
  --parameter cursor_api_key=<their-personal-key> \
  --parameter git_repo_url=https://github.com/your-org/your-repo
```

Or via the Coder web UI. Within a minute the workspace boots and
registers a worker named `coder-alice-my-cursor-worker` in Alice's
Cursor account.

## Step 6: Trigger sessions

From Slack, GitHub, or Linear, the developer includes `worker=` or
`machine=` plus their worker name:

- Slack: `@Cursor worker=coder-alice-my-cursor-worker fix the flaky test`
- GitHub: `@cursoragent worker=coder-alice-my-cursor-worker fix the flaky test`
- Linear: add `worker=coder-alice-my-cursor-worker` to the issue body.

The trigger must come from the linked Cursor account; only the
developer's own machines match.

Or from the Cloud Agents dashboard: pick the named machine from the
environment dropdown.

## Operate

### Logs

`cursor-agent` writes to `~/cursor-agent.log` inside each workspace.
Each developer tails their own.

### Sizing

There is no pool to size. Each developer adjusts their workspace
resources via the template's resource block or per-workspace
parameters.

### Rotation

Each developer rotates their own Cursor key:

1. Generate a new key at `cursor.com/dashboard/integrations`.
2. Update the workspace parameter via the Coder UI.
3. Restart the workspace.

### Upgrade `cursor-agent`

Same as Worker Pool: bump the version in the image, rebuild, push the
template. Existing workspaces upgrade on next restart.

## Common pitfalls

| Symptom                                                      | Cause and fix                                                                                                                                                                                                                                                       |
|--------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `worker=<name>` request rejected with "different repository" | The developer asked Cursor to run on `worker=foo` but `foo` was started in a checkout of a different repo. Start a new workspace for the target repo.                                                                                                               |
| Cursor UI doesn't show the worker                            | The personal API key in the workspace parameter doesn't match the Cursor account the developer triggers from. Confirm the same Cursor user owns both.                                                                                                               |
| Request runs on Cursor-managed infra instead                 | The trigger didn't include `worker=<name>` or `machine=<name>`. These are the only triggers that target a personal machine. `self_hosted=true` and `pool=<name>` target Worker Pool, not personal machines.                                                         |
| Multiple users want to share a workspace                     | Personal Workers is one-user-per-workspace by design. For shared inventory use [Worker Pool](./system-identity.md) (requires Cursor Enterprise).                                                                                                                    |
| Worker stays connected after the developer logs off          | Set `coder stop` on a TTL via the template, or document the expectation that developers stop their workspaces when done. Personal Workers has no `--idle-release-timeout` analogue because there's no pool draining; the workspace lifetime is the worker lifetime. |

## Where to next

- [Worker Pool](./system-identity.md): admin-operated central pool,
  requires Cursor Enterprise.
- [AI Governance Integration](./ai-governance.md): how Personal Workers
  affects AI Gateway coverage compared to Worker Pool.
- [Implementation Notes](./plan.md): the staged plan and open questions.
