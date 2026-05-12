# Setup

This page is the copy-and-go version of the [overview](./index.md). It walks
through publishing a Coder template that runs a Claude Code self-hosted
runner inside each developer's workspace, with no Coder product changes
required.

> [!NOTE]
> Self-hosted runners are in early access (EAP) from Anthropic. You will
> need a `BYOC_VERSION` and tarball URL from your Anthropic account team
> before you can complete this guide.

## Prerequisites

- A Coder deployment your developers already use.
- An Anthropic organization admin who can create self-hosted runner pools
  at `claude.ai → Settings → Claude Code → Self-hosted runner pools`.
- A workspace base image that can install the runner. The examples below
  assume a Debian or Ubuntu base; adjust package names for other distros.
- Outbound HTTPS access from the workspace to `api.anthropic.com` and to
  `storage.googleapis.com` (only needed during image build, to download the
  runner tarball).
- Coder admin access to publish a new template.

## Step 1: Create the pool in `claude.ai`

1. Sign in to `claude.ai` as an Anthropic org admin.
2. Go to **Settings → Claude Code → Self-hosted runner pools**.
3. Click **Create pool**, give it a name (for example `coder-workspaces`),
   and submit.
4. **Copy the pool secret.** It is displayed once and cannot be retrieved
   later. Put it in your existing secrets store (Vault, 1Password, AWS
   Secrets Manager, etc.).

## Step 2: Bake the runner into a workspace image

Anthropic's tarball contains one subdirectory per platform. On Linux x86_64
the relevant directory is `linux-x64`. The example `Dockerfile` below pins
to a specific `BYOC_VERSION`, installs the runner system-wide at
`/opt/claude/claude`, and sets the system-level Git identity required by
the runner.

```dockerfile
# Base on whatever your workspaces normally use.
FROM ghcr.io/your-org/base:latest

ARG BYOC_VERSION=2.1.97-byoc.9
ENV BYOC_VERSION=${BYOC_VERSION}

USER root

# Minimum runtime dependencies. Add language toolchains your sessions need
# (node, go, python, java, etc.).
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git jq tini openssh-client \
    && rm -rf /var/lib/apt/lists/*

# Anthropic-managed sessions use this identity. You can use your own bot
# identity instead. --system writes to /etc/gitconfig and applies to every
# user, including the workspace user.
RUN git config --system user.name "Claude" \
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

# Drop back to the workspace user expected by Coder.
USER coder
```

> [!TIP]
> Validate the binary in the image before publishing:
>
> ```bash
> docker run --rm your-image:tag claude self-hosted-runner --help
> ```

### Git push credentials

Coder already has a feature for this: [external auth](../../admin/external-auth/index.md).
When a workspace owner authenticates a configured provider (GitHub,
GitLab, Bitbucket, Azure DevOps, etc.), the Coder agent wires
`GIT_ASKPASS` automatically. `git push` inside the workspace and inside
the runner's child `claude` process both pick up the token with no extra
setup, and Coder refreshes the token in the background. You do not need
to bake credentials into the image or wire a credential helper.

Reference `coder_external_auth` in the template:

```hcl
data "coder_external_auth" "github" {
  id = "github"
}
```

For the dev or proof-of-concept case where external auth is not yet
configured on the Coder server, set `optional = true`:

```hcl
data "coder_external_auth" "github" {
  id       = "github"
  optional = true
}
```

The workspace still builds. Flip `optional` to `false` once the server
has `CODER_EXTERNAL_AUTH_0_*` set; Coder then prompts each workspace
owner to authenticate before creating the workspace.

If your Git host only accepts SSH, fall back to the patterns the runner
build already supports: mount an SSH key into the image, and pass
`--git-ssh-rewrite <host>` to the runner so `https://<host>/...` repo
URLs are rewritten to `git@<host>:...` before clone.

If checkouts will be owned by a different uid than the runner process,
keep the `safe.directory '*'` line in the `Dockerfile`. Otherwise drop
it.

## Step 3: Publish the Coder template

The template defines:

- A workspace that runs the runner as long as the workspace is up.
- A sensitive parameter for the pool secret (the developer pastes it once
  per workspace), or an injected env var if your platform pushes per-user
  secrets.
- A capacity parameter so power users can run more concurrent sessions.

The Terraform below is a minimal Docker-backed example you can adapt to
Kubernetes or your existing template. Replace the `coder_parameter` and
`docker_container` blocks with whatever your environment uses.

```hcl
terraform {
  required_providers {
    coder  = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

variable "pool_secret_default" {
  type        = string
  default     = ""
  description = "Optional. Inject the pool secret at template-build time."
  sensitive   = true
}

data "coder_parameter" "pool_secret" {
  name         = "pool_secret"
  display_name = "Claude Code pool secret"
  description  = "Pool secret from claude.ai → Settings → Claude Code → Self-hosted runner pools."
  type         = "string"
  default      = var.pool_secret_default
  mutable      = true
  ephemeral    = false
}

data "coder_parameter" "capacity" {
  name         = "capacity"
  display_name = "Concurrent sessions"
  description  = "Maximum sessions this workspace runs at once. The runner is locked to your user."
  type         = "number"
  default      = "4"
  mutable      = true
  validation { min = 1; max = 16 }
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"

  env = {
    CLAUDE_POOL_SECRET = data.coder_parameter.pool_secret.value
    CLAUDE_CAPACITY    = tostring(data.coder_parameter.capacity.value)
  }

  startup_script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "$HOME/.claude"
  EOT
}

resource "coder_script" "claude_runner" {
  agent_id     = coder_agent.main.id
  display_name = "Start Claude Code self-hosted runner"
  run_on_start = true

  script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail

    if [ -z "$${CLAUDE_POOL_SECRET:-}" ]; then
      echo "CLAUDE_POOL_SECRET is empty. Edit the workspace and paste the pool secret."
      exit 1
    fi

    POOL_SECRET_FILE=/etc/claude/pool-secret
    sudo install -d -m 0750 /etc/claude
    printf '%s' "$CLAUDE_POOL_SECRET" | sudo tee "$POOL_SECRET_FILE" >/dev/null
    sudo chmod 0400 "$POOL_SECRET_FILE"

    exec /usr/local/bin/claude self-hosted-runner \
      --pool-secret-file "$POOL_SECRET_FILE" \
      --capacity "$${CLAUDE_CAPACITY:-1}" \
      --log-file "$HOME/.claude/runner.log"
  EOT
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "your-org/claude-runner:latest"
  name  = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
  ]

  entrypoint = ["/bin/sh", "-c", coder_agent.main.init_script]

  # Optional: mount tools and secrets the runner needs.
  # volumes { ... }
}
```

> [!TIP]
> If your secrets platform can deliver per-user secrets to the workspace
> (for example, Vault Agent or a Kubernetes `Secret` mounted via a
> [coder workspace tag](../../admin/templates/extending-templates/parameters.md)),
> set `var.pool_secret_default` from that source and hide the parameter
> from end users by setting `mutable = false` and supplying the value at
> template-build time.

## Step 4: Verify

1. Create a workspace from the new template. Provide the pool secret (or
   confirm your platform injected it).
2. Open the workspace and tail the runner log:

   ```bash
   tail -f ~/.claude/runner.log
   ```

   You should see `runner registered with pool …` and `polling for work`.

3. In `claude.ai → Settings → Claude Code → Self-hosted runner pools`,
   confirm the runner appears under the pool within a few seconds.
4. Start a session at `claude.ai/code` and select the pool from the
   environment picker. The session should be assigned to your workspace
   within seconds. The session UI will show the same experience as an
   Anthropic-managed session.

## Step 5: Operate

- **Logs.** The runner writes to `~/.claude/runner.log` plus stderr (visible
  in the Coder workspace agent logs). Each session also writes a child
  debug log to `$TMPDIR/claude-code-debug-<sessionId>.txt` that the runner
  preserves on failure.
- **Restart on changes.** To pick up a new image, an updated pool secret,
  or a runner build update, stop and start the workspace. Coder restarts
  the `coder_script` block on workspace start, which restarts the runner
  with a fresh process and fresh filesystem.
- **Rotate the pool secret.** Create a new secret in the Anthropic admin UI,
  update the parameter or injected env var, then stop and start workspaces.
  Workspaces still holding the old secret will fail their next token
  refresh and exit; Coder restarts them with the new secret on next start.
- **Upgrade the runner.** Publish a new image with a newer `BYOC_VERSION`,
  bump the template version, and let developers update their workspaces on
  their normal cadence.

## Common pitfalls

| Symptom                                              | Cause and fix                                                                                                                                                                                        |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Runner never appears in the pool                     | Workspace cannot reach `api.anthropic.com`. Check workspace egress rules. The runner logs `[runner:fatal]` with the rejection reason on auth failure.                                                |
| Sessions fail immediately after pickup               | Missing Git credentials in the image, or a build tool the session expects is not installed. Open the session in `claude.ai/code` to see the error.                                                   |
| `git commit` fails with `Please tell me who you are` | The `user.name` / `user.email` was set with `git config --global` for a different user. Use `git config --system` in the `Dockerfile` so the identity applies regardless of the runtime user.        |
| Workspace restarts kill in-flight sessions           | Coder's default workspace stop runs the script's `SIGTERM` handler. Anthropic recommends a 60-second termination grace period; if your platform overrides that, raise it on the workspace container. |

## Next: more advanced setups

Once the basic flow is healthy, see [Plan](./plan.md) for staged
improvements that are still pure documentation and template work:
per-creator AWS or Vault credentials via a wrapper script, routing the
child `claude` process through Coder's AI Gateway, pinning permissions in
the workspace image, and using short-lived runner workspaces for fleet
pools.
