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

# The declarative AI sandbox shape.
#
# Two agents in one ordinary, human-owned workspace:
#
#   coder_agent.main  unbound. Keeps the owner's credentials. Runs the
#                     egress proxy and the script that builds the sandbox.
#   coder_agent.ai    ai_bound. The server binds it to a dedicated AI agent
#                     identity, withholds every ambient owner credential,
#                     and attributes its actions to that identity on behalf
#                     of the owner.
#
# The workspace itself is NOT AI-designated. That distinction is the point:
# designation would bind every agent including the host, which would starve
# the supervisor of the credentials and policy it needs to do its job.

variable "workspace_image" {
  description = <<-EOT
    Workspace image. It must carry the coder-sandbox binary and its
    microsandbox runtime. The default Coder images do not; see the README.
  EOT
  type        = string
  default     = "codercom/example-base:ubuntu"
}

variable "sandbox_image" {
  description = "OCI image booted inside the coder/sandbox microVM."
  type        = string
  default     = "ubuntu:latest"
}

variable "docker_socket" {
  description = "(Optional) Docker socket URI."
  type        = string
  default     = ""
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "sandbox_allow" {
  name         = "sandbox_allow"
  display_name = "Sandbox egress allowlist"
  description  = <<-EOT
    Extra hosts the sandboxed AI may reach, comma separated. The coderd host
    is always allowed so the bound agent can connect. A leading dot matches
    subdomains, for example .github.com. Everything else is denied.
  EOT
  type         = "string"
  default      = ""
  mutable      = true
}

locals {
  # coder-sandbox enforces the guest's egress itself, so the sandbox name is
  # the handle both the create and the destroy script use.
  sandbox_name = "coder-ai-${data.coder_workspace.me.id}"
}

# The host agent. Ordinary in every respect: full owner credentials, and it
# is the egress supervisor for the sandbox it builds.
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
# The server mints (or reuses) this workspace's AI agent identity at build
# completion and binds this agent row to it. From that moment the agent is
# denied the owner's user secrets, external auth tokens, and Git SSH key,
# and its API calls attribute to the AI identity on behalf of the owner.
#
# ai_bound is opt-in only and monotonic: setting it to false cannot unbind
# an agent, because in-workspace input must never widen a credential
# boundary.
resource "coder_agent" "ai" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  ai_bound = true

  # An administrator attestation about the boundary the startup script
  # builds. coder/sandbox routes the guest through its own recording proxy
  # with a deny-by-default allowlist, so "forced" is the honest claim here.
  # The platform records the claim; it does not verify it.
  egress_enforcement = "forced"
}

# Bring the sandbox up. This is an ordinary startup script: its content is
# versioned with the template and its output is surfaced as script logs.
#
# The agent supplies CODER_EGRESS_PROXY and CODER_SANDBOX_ID at exec time,
# and does not run this script until the egress proxy is listening. The
# bound agent's token comes from Terraform, because a declared agent has a
# token like any other.
resource "coder_script" "sandbox_up" {
  agent_id     = coder_agent.main.id
  display_name = "Start AI sandbox"
  icon         = "/icon/container.svg"
  run_on_start = true

  script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail
    export CODER_AI_SANDBOX_NAME="${local.sandbox_name}"
    export CODER_AI_SANDBOX_IMAGE="${var.sandbox_image}"
    export CODER_AI_SANDBOX_ALLOW="${data.coder_parameter.sandbox_allow.value}"
    export CODER_AI_AGENT_URL="$${CODER_AGENT_URL:-}"
    export CODER_AI_AGENT_TOKEN="${coder_agent.ai.token}"
    exec bash /opt/coder-ai/sandbox-up.sh
  EOT
}

# Tear it down on stop. Credentials are already inert by then, because the
# server revokes this build's keys on the stop transition; this script only
# reclaims the microVM.
resource "coder_script" "sandbox_down" {
  agent_id     = coder_agent.main.id
  display_name = "Stop AI sandbox"
  icon         = "/icon/container.svg"
  run_on_stop  = true

  script = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail
    export CODER_AI_SANDBOX_NAME="${local.sandbox_name}"
    exec bash /opt/coder-ai/sandbox-down.sh
  EOT
}

# Apps attach to the bound agent directly: this terminal opens inside the
# sandbox, not in the human's workspace.
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
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name

  # The microVM backend needs hardware virtualization.
  devices {
    host_path = "/dev/kvm"
  }

  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]

  # Only the host agent's token goes here. The bound agent's token reaches
  # the sandbox through the startup script, never through the host agent's
  # own environment.
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  # The sandbox scripts and the microsandbox runtime cache.
  volumes {
    container_path = "/opt/coder-ai"
    host_path      = abspath("${path.module}/scripts")
    read_only      = true
  }
}
