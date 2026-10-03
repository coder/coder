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

locals {
  username               = data.coder_workspace_owner.me.name
  chat_control_plane_url = replace(data.coder_workspace.me.access_url, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# -------------------------------------------------------------------
# Agent 1: Regular dev agent (user-facing, appears in the dashboard)
# -------------------------------------------------------------------
resource "coder_agent" "dev" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
  }

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }
}

# See https://registry.coder.com/modules/coder/code-server
module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/code-server/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.dev.id
  order    = 1
}

# -------------------------------------------------------------------
# Agent 2: Chat agent (designated for chatd-managed AI chat)
#
# This agent runs inside a bubblewrap (bwrap) sandbox. The entire
# agent process and all its children (tool calls, SSH sessions, etc.)
# execute in a restricted mount namespace. There is no escape path
# because the sandbox wraps the agent binary itself, not just the
# shell.
#
# The agent name "dev-coderd-chat" ends with the -coderd-chat suffix
# that tells chatd to route chats here. The dashboard still shows the
# agent, but the template reserves it for chatd-managed sessions rather
# than normal user interaction.
#
# NOTE: Terraform resource labels cannot contain hyphens, but the
# Coder provisioner uses the label as the agent name (and rejects
# underscores). To work around this, the resource label uses hyphens
# and all references go through the local.chat_agent indirection
# below.
# -------------------------------------------------------------------

# Terraform parses "coder_agent.dev-coderd-chat.X" as subtraction,
# so we capture the agent attributes in locals for clean references.
locals {
  # The resource block below uses a hyphenated label so the Coder
  # provisioner registers the agent name as "dev-coderd-chat".
  # These locals let the rest of the config reference its attributes
  # without Terraform misinterpreting the hyphens.
  chat_agent_init  = replace(coder_agent.dev-coderd-chat.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
  chat_agent_token = coder_agent.dev-coderd-chat.token
}

resource "coder_agent" "dev-coderd-chat" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  order          = 99
  startup_script = <<-EOT
    set -e
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
  }
}

# -------------------------------------------------------------------
# Docker image with bubblewrap pre-installed
# -------------------------------------------------------------------
resource "docker_image" "chat_sandbox" {
  name = "coder-chat-sandbox:latest"

  build {
    context    = "."
    dockerfile = "Dockerfile.chat"
  }
}

# -------------------------------------------------------------------
# Shared home volume
# -------------------------------------------------------------------
resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

# -------------------------------------------------------------------
# Container 1: Dev workspace (regular agent, no sandbox)
# -------------------------------------------------------------------
resource "docker_container" "dev" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-base:ubuntu"
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  entrypoint = [
    "sh", "-c",
    replace(coder_agent.dev.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
  ]
  env = ["CODER_AGENT_TOKEN=${coder_agent.dev.token}"]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}

# -------------------------------------------------------------------
# Container 2: Chat sandbox (agent runs inside bubblewrap)
#
# The entrypoint pipes the agent init script through bwrap-agent,
# which starts the entire agent binary inside a bwrap namespace.
# Every process the agent spawns (sh -c for tool calls, SSH
# sessions, etc.) inherits the restricted mount namespace:
#
#   - Read-only root filesystem (cannot modify system files)
#   - Read-write /home/coder (shared project files)
#   - Private /tmp (tmpfs scratch space)
#   - Shared network namespace with outbound TCP restricted to the
#     Coder control-plane endpoint used by the agent over IPv4 and IPv6
#
# Because the agent itself runs inside bwrap, there is no way for
# a tool call to escape the sandbox by invoking /bin/bash or any
# other binary directly. All binaries are inside the same namespace.
# -------------------------------------------------------------------
resource "docker_container" "chat" {
  count    = data.coder_workspace.me.start_count
  image    = docker_image.chat_sandbox.image_id
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}-chat"
  hostname = "${data.coder_workspace.me.name}-chat"

  # Capability budget:
  # - SYS_ADMIN: bwrap needs this to create mount namespaces.
  # - NET_ADMIN: the wrapper needs this to install iptables OUTPUT
  #   rules before entering bwrap.
  # - DAC_OVERRIDE: passed through to the sandbox so the agent
  #   (running as root) can read/write files owned by uid 1000 on
  #   the shared home volume without changing ownership.
  # - seccomp=unconfined: Docker's default seccomp profile blocks
  #   pivot_root, which bwrap uses during namespace setup.
  capabilities {
    add  = ["SYS_ADMIN", "NET_ADMIN", "DAC_OVERRIDE"]
    drop = ["ALL"]
  }
  security_opts = ["seccomp=unconfined"]

  # Wrap the init script through bwrap-agent so the agent binary
  # and all its children run inside the sandbox namespace.
  # The init script is base64-encoded to avoid nested shell quoting
  # issues, then decoded and executed at container startup.
  entrypoint = [
    "sh", "-c",
    "echo ${base64encode(local.chat_agent_init)} | base64 -d > /tmp/coder-init.sh && chmod +x /tmp/coder-init.sh && exec bwrap-agent sh /tmp/coder-init.sh"
  ]
  env = [
    "CODER_AGENT_TOKEN=${local.chat_agent_token}",
    "CODER_SANDBOX_CONTROL_PLANE_URL=${local.chat_control_plane_url}",
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

  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
