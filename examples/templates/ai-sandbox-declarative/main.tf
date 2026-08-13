terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

# An AI agent confined to a sibling container, declared entirely in Terraform.
#
# Two agents in one ordinary, human-owned workspace:
#
#   coder_agent.main  unbound. Keeps the owner's credentials. Runs the egress
#                     proxy and the scripts that build and reclaim the sandbox.
#   coder_agent.ai    ai_bound. The server binds it to a dedicated AI agent
#                     identity, withholds every ambient owner credential, and
#                     attributes its actions to that identity on behalf of the
#                     owner.
#
# The workspace itself is NOT AI-designated, and that distinction is the whole
# point: designation binds every agent including the host, which would starve
# the supervisor of the credentials and policy it needs to do its job.

variable "docker_socket" {
  description = "(Optional) Docker socket URI."
  type        = string
  default     = ""
}

variable "workspace_image" {
  description = "Workspace image. Must contain the Docker CLI."
  type        = string
  default     = "codercom/enterprise-base:ubuntu"
}

variable "sandbox_image" {
  description = "Image for the sandbox container that holds the AI agent."
  type        = string
  default     = "codercom/enterprise-base:ubuntu"
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  # The scripts need this to attach the workspace to the sandbox's internal
  # network. Terraform knows it; discovering it from inside the container
  # would be fragile.
  workspace_container = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # The workspace image runs as an unprivileged user. /opt is normally owned
  # by root, while /tmp is writable and recreated with the workspace container.
  staging_dir = "/tmp/coder-ai"

  # The Docker provider talks to the host daemon. A bind mount whose host_path
  # points at path.module would therefore ask the daemon to read a path in the
  # provisioner's filesystem, where the daemon usually cannot see it. Stage
  # the scripts and agent init through the container entrypoint instead.
  agent_init = replace(
    coder_agent.main.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )
}

# The host agent: ordinary in every respect. It holds the owner's credentials
# and is the egress supervisor for the sandbox it builds.
resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  startup_script = <<-EOT
    set -e
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~ 2>/dev/null || true
      touch ~/.init_done
    fi
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }
}

# The AI's agent. ai_bound is the entire security-relevant declaration.
#
# At build completion the server resolves this workspace's AI agent identity,
# creating it on first use, and binds this agent row to it. From that moment
# the agent is denied the owner's user secrets, external auth tokens, and Git
# SSH key, and its API calls attribute to the AI identity on behalf of the
# owner.
#
# ai_bound is opt-in only and monotonic: setting it to false cannot unbind an
# agent, because in-workspace input must never widen a credential boundary.
resource "coder_agent" "ai" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  ai_bound = true

  # An administrator attestation about the boundary the startup script builds.
  # The sandbox joins a Docker network created with --internal, which has no
  # route off the host, so the workspace-side proxy is the only path out.
  # "forced" is therefore an honest claim here. The platform records it; it
  # does not verify it.
  egress_enforcement = "forced"
}

# Build the sandbox. This is an ordinary startup script: its content is
# versioned with the template and its output is surfaced as script logs.
#
# The agent injects CODER_EGRESS_PROXY and CODER_SANDBOX_ID at exec time and
# does not run this script until the egress proxy is listening, so there is no
# window in which the sandbox runs unconfined.
resource "coder_script" "sandbox_up" {
  agent_id     = coder_agent.main.id
  display_name = "Start AI sandbox"
  icon         = "/icon/docker.png"
  run_on_start = true

  script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail
    export CODER_AI_SANDBOX_IMAGE="${var.sandbox_image}"
    export CODER_AI_SANDBOX_WORKSPACE="${local.workspace_container}"
    export CODER_AI_AGENT_URL="$${CODER_AGENT_URL:-}"
    export CODER_AI_AGENT_TOKEN="${coder_agent.ai.token}"
    exec bash ${local.staging_dir}/sandbox-up.sh
  EOT
}

resource "coder_script" "sandbox_down" {
  agent_id     = coder_agent.main.id
  display_name = "Stop AI sandbox"
  icon         = "/icon/docker.png"
  run_on_stop  = true

  script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail
    export CODER_AI_SANDBOX_WORKSPACE="${local.workspace_container}"
    exec bash ${local.staging_dir}/sandbox-down.sh
  EOT
}

# Apps attach to the bound agent directly, so this terminal opens inside the
# sandbox rather than in the human's workspace.
resource "coder_app" "sandbox_terminal" {
  agent_id     = coder_agent.ai.id
  slug         = "ai-terminal"
  display_name = "AI Sandbox"
  icon         = "/icon/terminal.svg"
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = var.workspace_image
  name     = local.workspace_container
  hostname = data.coder_workspace.me.name

  # Stage the sandbox scripts and the agent init script inside the workspace
  # container before the agent starts. Everything is base64 encoded so script
  # contents cannot break shell quoting. Do not bind-mount path.module here:
  # Docker resolves host_path from the daemon's filesystem, not from the
  # provisioner's filesystem.
  entrypoint = ["sh", "-c", <<-EOT
    set -eu
    mkdir -p ${local.staging_dir}
    printf '%s' '${base64encode(file("${path.module}/scripts/sandbox-up.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-up.sh
    printf '%s' '${base64encode(file("${path.module}/scripts/sandbox-down.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-down.sh
    printf '%s' '${base64encode(local.agent_init)}' | base64 -d > ${local.staging_dir}/init.sh
    chmod +x ${local.staging_dir}/sandbox-up.sh ${local.staging_dir}/sandbox-down.sh ${local.staging_dir}/init.sh
    exec sh ${local.staging_dir}/init.sh
  EOT
  ]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",

    # Run the egress proxy without a platform-managed sandbox. The template
    # owns the sandbox through coder_agent.ai and the scripts above; the agent
    # contributes only the proxy, and the policy it enforces.
    "CODER_AI_EGRESS_PROXY=true",

    # The proxy must bind an address the sandbox container can reach, so it
    # cannot use the workspace's loopback.
    "CODER_AI_SANDBOX_PROXY_ADDRESS=0.0.0.0:13337",
    "CODER_AI_SANDBOX_EGRESS_ENFORCEMENT=forced",
    "CODER_AI_SANDBOX_NAME=${lower(data.coder_workspace.me.name)}",
  ]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  # The sandbox scripts drive Docker on the host, creating the sandbox as a
  # SIBLING container rather than a nested one. This grants the workspace
  # control of the host Docker daemon, which is a deliberate trust decision:
  # acceptable for a demo, and the reason rootless Podman is the production
  # direction. Note the AI cannot reach this socket: it lives in the
  # workspace, and the sandbox is on an internal network with no route here.
  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
    read_only      = false
  }
}
