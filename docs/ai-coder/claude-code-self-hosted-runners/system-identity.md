# System identity: a pool of bot runners

Publish a Coder template that runs a pool of Claude Code self-hosted
runners under a **system (bot) identity**. Coder maintains warm runner
workspaces; Anthropic's pool scheduler picks one when a session arrives
and locks it to the developer who started the session. The workspace,
the git push credential, and the commit author are all the same bot
identity, fleet-wide.

<img src="../../images/guides/claude-code-self-hosted-runners/system-identity-flow.svg" alt="Coder maintains a pool of warm prebuilt workspaces. When an Anthropic session arrives, the pool scheduler picks one and locks it to the developer. When work drains, the workspace deletes itself and the prebuild reconciler queues a replacement." />

> [!TIP]
> You can run this on Coder today. The primitives this guide depends
> on (templates, prebuilds, sensitive Terraform variables, agent
> metadata, self-eviction via the Coder API) are all already there.
> The recipe is a good fit for a scoped POC, a single bot identity
> over a small set of repos, or a fleet that runs Claude Code as a
> build agent.
>
> The routing layer described in [User identity](./user-identity.md)
> will lift both limitations below: it spawns a workspace per
> Anthropic user on demand instead of capping at `instances`, and the
> workspace owner becomes the human so external auth, the audit log,
> and the commit author can all use the developer's identity. The
> template, image, and pool you publish here stay; the routing layer
> goes in front.

## Limitations

Read these first; the rest of this page assumes they fit your team.

- **Fixed pool size, one Anthropic user per workspace.** Coder keeps
  exactly `instances = N` service-account-owned workspaces; each runner
  locks 1:1 to whichever Anthropic user it first serves. The
  `N+1`th concurrent user waits in Anthropic's queue. Bumping
  `instances` is the only lever, because every workspace is the same
  service account, not the human. [User identity](./user-identity.md)
  is what lifts this by spawning a workspace per Anthropic user on
  demand.
- **Bot identity end to end.** Commit author, git push token, workspace
  owner, and Coder audit log entries are all the prebuilds service
  account. The per-human signal is the Anthropic session URL appended
  to each commit as a trailer; the rest lives in Anthropic's session
  log. Coder external auth is unavailable inside prebuilt workspaces,
  so the bot git credential ships as a Terraform variable.
- **Stalled sessions are dropped.** Once the runner's active session
  count hits zero it drains; half-finished working trees are lost.
  Park long-running interactive sessions in a regular Coder workspace
  instead. The prebuild preset also recycles each warm runner after
  `expiration_policy.ttl` (8 hours in the example below), so a runner
  that locked but stayed idle is still reclaimed on schedule.
- **`--capacity` is per-runner parallelism, not pool concurrency.**
  Each runner serves up to `--capacity` sessions for the one locked
  user. Cross-user concurrency is bounded by `instances`, not
  `--capacity`.

If the first two are blockers for your team, wait for
[User identity](./user-identity.md).

## What you build in this guide

- One Coder template that bakes the runner binary, runs it as long as
  the workspace is up, and deletes the workspace from the inside after
  the runner drains.
- One Coder preset on that template configured with
  `prebuilds { instances = N }` so Coder keeps N warm runners ready for
  the Anthropic pool to claim.
- Three sensitive template variables: the Anthropic pool secret, a git
  bot credential, and a Coder service-account token the workspace uses
  to delete itself.
- Agent metadata that surfaces the currently locked Anthropic user, the
  in-flight session count, the runner ID, and the last-poll age on the
  Coder workspace page.

The result is a self-healing pool. When the Anthropic pool scheduler
routes a session, one of your warm workspaces locks to that user, serves
their sessions, drains, and deletes itself. The prebuild reconciler
notices the deficit and queues a replacement.

## Prerequisites

- A Coder deployment with **Coder Premium** (required for the prebuilds
  primitive).
- An Anthropic organization admin who can create self-hosted runner pools
  at `claude.ai > Settings > Claude Code > Self-hosted runner pools`.
- A workspace base image that can install the runner. The examples below
  assume a Debian or Ubuntu base; adjust package names for other distros.
- Outbound HTTPS access from the workspace to `api.anthropic.com` and to
  `storage.googleapis.com` (only needed during image build, to download
  the runner tarball).
- Coder admin access to publish a new template, create a service account
  user, and mint a long-lived API token for it.
- A bot identity on your git host (a GitHub bot user with a personal
  access token, an SSH deploy key, or equivalent on GitLab or Bitbucket).

## Identity model

Every commit and push from this pool is the **bot identity**, not the
human. This is intentional:

| Layer            | Identity                                                            |
|------------------|---------------------------------------------------------------------|
| Coder workspace  | Owned by Coder's prebuilds service account                          |
| Git author       | Bot (set in `/etc/gitconfig` in the image)                          |
| Git push token   | Bot PAT or SSH deploy key (sensitive template variable)             |
| Anthropic runner | Locked to the Anthropic user that sent the first session            |
| Commit trailer   | Session URL appended by Claude Code (this is your per-human signal) |

If you need per-user git attribution, ssh-signed commits, or Coder audit
log entries attributed to the human, see
[User identity](./user-identity.md).

## Step 1: Create the Anthropic pool

1. Sign in to `claude.ai` as an Anthropic org admin.
2. Go to **Settings > Claude Code > Self-hosted runner pools**.
3. Click **Create pool**, give it a name (for example `coder-workspaces`),
   and submit.
4. **Copy the pool secret.** It is displayed once and cannot be retrieved
   later. Put it in your existing secrets store (Vault, 1Password, AWS
   Secrets Manager, etc.).

## Step 2: Bake the runner into a workspace image

Anthropic's tarball contains one subdirectory per platform. On Linux
x86_64 the relevant directory is `linux-x64`. The `Dockerfile` below
pins to a specific `BYOC_VERSION`, installs the runner system-wide at
`/opt/claude/claude`, sets the system-level git identity required by the
runner, and creates `/workspace` owned by the workspace user.

```dockerfile
# Base on whatever your workspaces normally use.
FROM ghcr.io/your-org/base:latest

ARG BYOC_VERSION=2.1.97-byoc.9
ENV BYOC_VERSION=${BYOC_VERSION}

USER root

# Minimum runtime dependencies. Add language toolchains your sessions
# need (node, go, python, java, etc.). The Coder CLI is downloaded at
# workspace start by coder_script.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git jq tini openssh-client \
    && rm -rf /var/lib/apt/lists/*

# Anthropic-managed sessions use this identity. You can use your own bot
# identity instead; just keep it system-wide so it applies regardless of
# which user the runner process runs as.
RUN git config --system user.name  "Claude" \
 && git config --system user.email "noreply@anthropic.com" \
 && git config --system --add safe.directory '*'

# Pin and install the self-hosted runner binary.
RUN set -eux; \
    install -d /opt/claude; \
    curl -fsSL \
      "https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/byoc/releases/${BYOC_VERSION}/claude-byoc-${BYOC_VERSION}-all.tar.gz" \
      | tar -xz -C /opt/claude --strip-components=1 linux-x64; \
    ln -sf /opt/claude/claude /usr/local/bin/claude; \
    /usr/local/bin/claude --version

# Repository checkout root used by the self-hosted runner. The runner
# defaults to `--base-dir /workspace` and will `mkdir` it on the first
# session. Without this line the child claude (running as `coder`)
# fails with: EACCES: permission denied, mkdir '/workspace'.
RUN install -d -o coder -g coder /workspace

# Drop back to the workspace user expected by Coder.
USER coder
```

Validate the binary before publishing:

```bash
docker run --rm your-image:tag claude self-hosted-runner --help
```

## Step 3: Create the Coder service account and token

The workspace uses a Coder API token to delete itself when the runner
drains. Create a dedicated service account so that token's scope is
explicit and rotatable.

1. As a Coder admin, create the service account user:

   ```bash
   coder users create \
     --email svc-claude-delete@coder.local \
     --username svc-claude-delete \
     --password "$(openssl rand -base64 24)"
   ```

2. Grant the user the minimum role that includes `delete:workspace` on
   workspaces it does not own. The built-in `owner` role works for the
   pilot; you can scope further with a custom role once you are
   comfortable with the recipe:

   ```bash
   coder users edit-roles svc-claude-delete --roles owner
   ```

3. Mint a long-lived token for that user and store it as
   `CODER_DELETE_TOKEN` in your secrets store:

   ```bash
   coder tokens create \
     --user svc-claude-delete \
     --name claude-self-eviction \
     --lifetime 720h
   ```

> [!TIP]
> Treat the delete token like the pool secret: rotate it on a schedule
> by minting a new one, re-pushing the template with the new value,
> and revoking the old one.

## Step 4: Mint the bot git credential

Pick one of:

- **GitHub bot PAT.** Create a GitHub bot user (or use an existing
  organization bot), grant it write access to the repos you want
  sessions to push to, and mint a PAT scoped to those repos.
- **SSH deploy key.** Generate a key pair, install the public half on
  your git host with push permission, keep the private half for the
  template variable below.

The PAT path is simpler; the deploy-key path scopes better. Either works.

Store the credential as `GIT_BOT_TOKEN` (PAT) or `GIT_BOT_SSH_KEY` (SSH).

## Step 5: Publish the Coder template

The template defines:

- A workspace that runs the runner from `coder_script`, then deletes
  itself when the runner exits.
- Three sensitive variables: `pool_secret`, `git_bot_token`, and
  `coder_delete_token`.
- A `coder_workspace_preset` with `prebuilds { instances = N }` so Coder
  keeps N warm runners ready for the Anthropic pool to claim.
- `coder_agent.metadata` blocks that scrape the runner's `/healthz` and
  `/metrics` and surface the runner state on the workspace page.

The Terraform below is a minimal Docker-backed example. Adapt to
Kubernetes or your existing template by replacing the `docker_*` blocks.

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

# === Sensitive fleet credentials ===
# All three are set once with `coder templates push --variable name=...`
# and rotate by re-pushing the template.

variable "pool_secret" {
  type        = string
  description = "Claude Code self-hosted runner pool secret (from claude.ai)."
  sensitive   = true
}

variable "git_bot_token" {
  type        = string
  description = "Git PAT for the bot identity. Used by GIT_ASKPASS for pushes."
  sensitive   = true
}

variable "coder_delete_token" {
  type        = string
  description = "Coder API token used by the workspace to delete itself on drain."
  sensitive   = true
}

# === Workspace-shaped parameters ===

data "coder_parameter" "capacity" {
  name         = "capacity"
  display_name = "Concurrent sessions"
  description  = "Maximum sessions this runner serves at once."
  type         = "number"
  default      = "4"
  mutable      = true
  validation { min = 1, max = 16 }
}

# === Prebuild pool ===
# Coder keeps `instances` warm runners alive. When one drains and
# self-evicts, the reconciler queues a replacement so the pool stays at
# this count. Size to your expected peak concurrent Anthropic users.

data "coder_workspace_preset" "warm_runner" {
  name = "Warm runner"
  parameters = {
    capacity = "4"
  }
  prebuilds {
    instances = 5
    expiration_policy {
      ttl = 28800 # 8h: recycle warm runners past this even if unused.
    }
  }
}

# === Agent and runner ===

resource "coder_agent" "main" {
  arch                    = data.coder_provisioner.me.arch
  os                      = "linux"
  dir                     = "/home/coder"
  startup_script_behavior = "non-blocking"

  env = {
    CLAUDE_POOL_SECRET = var.pool_secret
    CLAUDE_CAPACITY    = tostring(data.coder_parameter.capacity.value)
    GIT_BOT_TOKEN      = var.git_bot_token
    CODER_DELETE_TOKEN = var.coder_delete_token
    CODER_WORKSPACE_ID = data.coder_workspace.me.id
  }

  startup_script = <<-EOT
    set -euo pipefail
    mkdir -p "$HOME/.claude"

    # Wire git push credentials for the bot identity. GIT_ASKPASS is a
    # one-line script that prints the token whenever git needs a password.
    if [ -n "$${GIT_BOT_TOKEN:-}" ]; then
      install -d -m 0700 "$HOME/.git-creds"
      cat > "$HOME/.git-creds/askpass.sh" <<'ASK'
    #!/bin/sh
    printf '%s' "$GIT_BOT_TOKEN"
    ASK
      chmod 0500 "$HOME/.git-creds/askpass.sh"
      git config --global core.askPass "$HOME/.git-creds/askpass.sh"
      git config --global credential.helper ''
    fi

    # Symlink the agent-installed Coder CLI onto a stable path so the
    # coder_script can call it on self-eviction without hunting for it
    # under /tmp/coder.*. The agent installs the CLI before run_on_start
    # fires.
    install -d "$HOME/.local/bin"
    coder_bin=$(find /tmp -maxdepth 2 -type f -name coder -executable 2>/dev/null | head -n1 || true)
    if [ -n "$coder_bin" ]; then
      ln -sf "$coder_bin" "$HOME/.local/bin/coder"
    fi
  EOT

  metadata {
    display_name = "Lock status"
    key          = "0_lock_status"
    interval     = 10
    timeout      = 5
    # The runner exposes `claude_code_self_hosted_runner_locked_account` as
    # a bare gauge (no labels), so we can only tell locked from unlocked,
    # not which user the runner is locked to. The locked user is visible
    # in `claude.ai > Self-hosted runner pools` and in Anthropic's session
    # log.
    script       = <<-EOT
      val=$(curl -fsS http://127.0.0.1:8080/metrics 2>/dev/null \
        | awk '/^claude_code_self_hosted_runner_locked_account[[:space:]]/ {print $2; exit}')
      if [ "$val" = "1" ]; then printf 'locked'
      else printf 'unlocked'; fi
    EOT
  }

  metadata {
    display_name = "Active sessions"
    key          = "1_active_sessions"
    interval     = 5
    timeout      = 5
    script       = <<-EOT
      active=$(curl -fsS http://127.0.0.1:8080/healthz 2>/dev/null \
        | jq -r '.active_sessions // empty')
      if [ -z "$active" ]; then echo '?'; exit 0; fi
      printf '%s / %s' "$active" "${CLAUDE_CAPACITY:-1}"
    EOT
  }

  metadata {
    display_name = "Runner ID"
    key          = "2_runner_id"
    interval     = 30
    timeout      = 5
    script       = <<-EOT
      curl -fsS http://127.0.0.1:8080/healthz 2>/dev/null \
        | jq -r '.runner_id // "(starting)"'
    EOT
  }

  metadata {
    display_name = "Last Anthropic poll"
    key          = "3_last_poll"
    interval     = 15
    timeout      = 5
    script       = <<-EOT
      age=$(curl -fsS http://127.0.0.1:8080/healthz 2>/dev/null \
        | jq -r '.last_poll_age_ms // empty')
      if [ -z "$age" ]; then echo '?'; exit 0; fi
      if [ "$age" -lt 30000 ]; then printf 'ok (%sms ago)' "$age"
      else printf 'stale (%ss ago)' $((age/1000)); fi
    EOT
  }
}

# coder_script spawns the runner as a detached supervisor and exits
# within a second. The supervisor runs the runner in the foreground,
# then calls 'coder delete' to self-evict so the prebuild reconciler
# refills the pool. Detaching is important: if we foregrounded the
# runner inside coder_script the workspace UI would show '1 script
# running' for the lifetime of the workspace, since the agent treats
# a coder_script as 'Running' while its process is alive.

resource "coder_script" "claude_runner" {
  agent_id           = coder_agent.main.id
  display_name       = "Claude self-hosted runner"
  run_on_start       = true
  start_blocks_login = false
  icon               = "/icon/code.svg"

  script = <<-EOT
    set -euo pipefail

    if [ -z "$${CLAUDE_POOL_SECRET:-}" ]; then
      echo "CLAUDE_POOL_SECRET is empty. Re-push the template with --variable pool_secret=..."
      exit 1
    fi

    POOL_SECRET_FILE="$HOME/.claude/pool-secret"
    install -d -m 0700 "$HOME/.claude"
    rm -f "$POOL_SECRET_FILE"
    umask 077
    printf '%s' "$CLAUDE_POOL_SECRET" > "$POOL_SECRET_FILE"
    chmod 0400 "$POOL_SECRET_FILE"

    # Resolve the coder CLI once, before detaching.
    coder_cmd="$HOME/.local/bin/coder"
    if [ ! -x "$coder_cmd" ]; then
      coder_cmd=$(find /tmp -maxdepth 2 -type f -name coder -executable 2>/dev/null | head -n1 || true)
    fi

    # Write the supervisor script. The supervisor runs the runner in the
    # foreground; on runner exit it self-evicts so the prebuild reconciler
    # refills the pool. CODER_URL must be set so the CLI knows which
    # server to talk to; a fresh workspace has no global Coder CLI config.
    SUPERVISOR="$HOME/.claude/supervisor.sh"
    cat > "$SUPERVISOR" <<SUP
    #!/usr/bin/env bash
    set -uo pipefail
    exec >>"$HOME/.claude/supervisor.log" 2>&1
    echo "[supervisor] start \$(date -Is)"

    /usr/local/bin/claude self-hosted-runner \
      --pool-secret-file "$POOL_SECRET_FILE" \
      --capacity        "$${CLAUDE_CAPACITY:-1}" \
      --log-file        "$HOME/.claude/runner.log"
    echo "[supervisor] runner exited rc=\$? \$(date -Is)"

    if [ -z "$${CODER_DELETE_TOKEN:-}" ] || [ -z "$coder_cmd" ] || [ ! -x "$coder_cmd" ]; then
      echo "[supervisor] skipping self-eviction (token or coder CLI missing)."
      exit 0
    fi
    CODER_URL="$CODER_AGENT_URL" \
      CODER_SESSION_TOKEN="$CODER_DELETE_TOKEN" \
      "$coder_cmd" delete --yes "$CODER_WORKSPACE_ID" \
      || echo "[supervisor] self-eviction failed (continuing)."
    SUP
    chmod 0700 "$SUPERVISOR"

    # Detach with setsid + nohup so the supervisor survives this script
    # exiting and ignores SIGHUP. The supervisor reopens stdout/stderr
    # to a logfile via 'exec >>...' above; we point its standard fds at
    # /dev/null here.
    setsid nohup "$SUPERVISOR" </dev/null >/dev/null 2>&1 &
    disown

    echo "Runner spawned as detached supervisor. See ~/.claude/supervisor.log."
  EOT
}

# === Docker resources ===

resource "docker_volume" "home" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle { ignore_changes = all }
}

resource "docker_image" "runner" {
  name = "your-org/claude-runner:latest"
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = docker_image.runner.name
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

> [!NOTE]
> The `coder` CLI must be on PATH inside the workspace for self-eviction
> to work. Coder's `enterprise-base` images include it; if you build
> from a stock distribution base, the agent downloads it on first start
> and the `startup_script` symlinks it onto `~/.local/bin/coder` so the
> `coder_script` can find it.
>
> Two settings let Coder show the workspace as "ready" with 0 running
> scripts while the long-lived runner is still polling Anthropic:
>
> - `startup_script_behavior = "non-blocking"` on the agent: lets
>   the agent flip to "ready" without waiting on `startup_script`.
> - **Detached supervisor** in `coder_script`: the script writes a
>   `supervisor.sh`, launches it with `setsid nohup ... &`, and exits.
>   The supervisor (re-parented to PID 1) runs the runner in the
>   foreground for the workspace's life, then runs `coder delete` when
>   the runner drains. Without this detach the agent treats
>   `coder_script` as "Running" the whole time, so the workspace page
>   shows "1 script running" and never reaches "ready".

## Step 6: Push the template

Push the template with all three variables set:

```bash
coder templates push claude-self-hosted-runner \
  --variable pool_secret="$(cat path/to/pool-secret)" \
  --variable git_bot_token="$(cat path/to/git-bot-token)" \
  --variable coder_delete_token="$(cat path/to/coder-delete-token)" \
  --yes
```

Within a few seconds, Coder spawns `instances` prebuilt workspaces. Each
one runs the runner, registers with the Anthropic pool, and starts
polling.

## Step 7: Verify

1. In Coder's admin UI, open **Workspaces > All** and filter by the
   prebuilds owner. You should see `instances` workspaces named
   `prebuild-...`, each one running, with metadata showing:
   - `Lock status: unlocked`
   - `Active sessions: 0 / 4`
   - `Runner ID: ccrunner_...`
   - `Last Anthropic poll: ok (...ms ago)`
2. In `claude.ai > Settings > Claude Code > Self-hosted runner pools`,
   confirm the same number of runners appear under the pool.
3. Start a session at `claude.ai/code` and select the pool. Within
   seconds, **one** of the Coder prebuilds flips its `Lock status`
   metadata to `locked` and `Active sessions` bumps to `1 / 4`. The
   Anthropic user the runner is locked to is visible in
   `claude.ai > Self-hosted runner pools`, not on the Coder workspace
   page (the runner does not expose the locked user's email over its
   local `/metrics` endpoint).
4. Send a few more messages or open additional sessions. Watch the
   active-session count climb (up to `--capacity`).
5. Let all sessions finish. The runner drains, the workspace
   self-evicts, and the prebuild reconciler queues a replacement
   workspace within a minute.

## Operate

### Logs

The runner writes to `~/.claude/runner.log` plus stderr (visible in the
Coder workspace agent logs). Each session also writes a child debug log
to `$TMPDIR/claude-code-debug-<sessionId>.txt` that the runner preserves
on failure.

### Pool sizing

Pick `instances = N` based on concurrent users. Each runner serves one
Anthropic user at a time, so the floor is "however many users you
expect to be active simultaneously." Bump if your queue depth shows
backlog at Anthropic; trim if warm runners sit idle for hours.

### Rotation

| Secret               | Rotate by                                                                                                                                                                                                                |
|----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `pool_secret`        | Create a new pool secret in `claude.ai`, re-push the template with `--variable pool_secret=...`, revoke the old one. Active runners holding the old secret exit on next token refresh and the reconciler replaces them. |
| `git_bot_token`      | Mint a new PAT, re-push the template, revoke the old PAT. Already-running runners use the old token until they drain.                                                                                                    |
| `coder_delete_token` | Generate a new token for the service account, re-push the template, delete the old token.                                                                                                                                |

### Upgrade the runner binary

Bump `BYOC_VERSION` in the `Dockerfile`, rebuild the image, push the
template. New prebuilds come up on the new version. Existing prebuilds
serve sessions until drain, then self-evict; replacements come up on
the new image.

### Reading the runner state

The `coder_agent.metadata` blocks already surface the most useful state.
For deeper debugging:

```bash
# Inside the workspace:
curl -s http://127.0.0.1:8080/healthz | jq .
curl -s http://127.0.0.1:8080/metrics | grep claude_code_self_hosted
tail -f ~/.claude/runner.log
```

## Common pitfalls

| Symptom                                                          | Cause and fix                                                                                                                                                                                                                                                                              |
|------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `EACCES: permission denied, mkdir '/workspace'`                  | The image is missing the `install -d -o coder -g coder /workspace` step. Sessions fail on first checkout. Fix in the `Dockerfile`, re-push the template, and let prebuilds replace warm runners.                                                                                           |
| Prebuilds never come up                                          | Confirm your deployment is on **Coder Premium** (prebuilds is an enterprise feature). Check `coder server` logs for `prebuilds` errors.                                                                                                                                                    |
| Runner appears in pool but sessions stay queued                  | The runner is locked to an Anthropic account that does not match the user trying to send a session. Each runner serves one Anthropic user at a time. Either wait for it to drain, add more `instances`, or wait for [User identity](./user-identity.md) which pre-locks per Anthropic user. |
| Self-eviction logs `You are not logged in`                       | The `coder_script` is missing `CODER_URL=$CODER_AGENT_URL` on the `coder delete` line. A freshly spawned workspace has no global Coder CLI config, so the CLI cannot guess the server URL. Add the line and re-push the template.                                                          |
| Workspace does not self-evict on drain                           | `CODER_DELETE_TOKEN` is wrong or the service account lacks `delete:workspace`. The workspace will sit idle until Coder's TTL reaps it. Fix the token or grant the role and re-push the template. As a backstop, set a workspace TTL on the template.                                       |
| `git push` fails with 403 / Permission denied                    | `git_bot_token` is missing required repo scope, or the bot identity does not have push access to the repo the session is working in. Confirm the bot's permissions on the git host.                                                                                                        |
| Workspace page shows "startup scripts are still running" forever | Expected without the detached supervisor. The runner is intentionally long-lived; the agent treats a `coder_script` as `Running` while its process is alive. Fix by spawning the runner as a detached `setsid nohup` supervisor inside `coder_script` and pairing it with `startup_script_behavior = "non-blocking"` on the agent. With both, the script exits within a second and the workspace flips to "ready".                              |
| Lock status shows `unlocked` but Active sessions shows `1 / 4`   | Expected with current Anthropic BYOC versions: the `claude_code_self_hosted_runner_locked_account` metric is a bare gauge with no labels, and the runner does not expose the locked user's email over its local endpoints. The locked user is visible only in `claude.ai > Self-hosted runner pools` and in Anthropic's session log. |
| Metadata shows `?` for active sessions                           | The runner has not started yet, the `/healthz` port is firewalled inside the container, or `jq` is missing from the image. Check `tail -f ~/.claude/runner.log` and `apt list --installed jq`.                                                                                              |

## Where to next

- [User identity](./user-identity.md): the user-identity follow-on.
  On the Coder + Anthropic roadmap; not yet available.
- [Implementation notes](./plan.md): the staged plan, the sub-stages
  within system identity (per-creator credentials, AI Gateway routing,
  custom checkout, tool allowlists), and the open questions tracked
  alongside this delivery.
