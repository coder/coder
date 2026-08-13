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
# Terraform owns both containers and both networks. The host workspace carries
# an ordinary unbound agent and the egress proxy. The sandbox carries the
# ai_bound agent and joins only an internal network, which has no route to the
# internet. The workspace joins both networks and exposes the proxy to the
# sandbox under a fixed network alias.

variable "docker_socket" {
  description = "(Optional) Docker socket URI used by the provisioner."
  type        = string
  default     = ""
}

variable "workspace_image" {
  description = "Image for the human workspace container."
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
  workspace_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  sandbox_name   = "${local.workspace_name}-ai"

  # Development access URLs commonly contain localhost. Inside a container,
  # that name points back at the container, so both agent init scripts use the
  # Docker host gateway instead. The sandbox itself has no route to that gateway;
  # its HTTP(S) proxy variables cause the host agent's proxy to make the control
  # plane connection on its behalf.
  host_agent_init = replace(
    coder_agent.main.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )
  ai_agent_init = replace(
    coder_agent.ai.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )
}

# The host agent is ordinary and unbound. It keeps the owner's credentials and
# is the egress supervisor for the sandbox.
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

# This one declaration activates the AI security policy. At build completion,
# coderd resolves the workspace-origin AI identity and binds only this agent.
# The workspace remains human-owned and undesignated, so coder_agent.main keeps
# the owner's credentials and can supervise the sandbox.
resource "coder_agent" "ai" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  ai_bound = true

  # The sandbox joins only an internal Docker network, so every successful
  # egress path is forced through the host proxy. Coder records this as an
  # administrator attestation; it does not inspect the Docker network itself.
  egress_enforcement = "forced"
}

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

# The workspace's ordinary network carries control-plane and internet traffic.
resource "docker_network" "workspace" {
  count = data.coder_workspace.me.start_count
  name  = "${local.workspace_name}-network"
}

# The sandbox network has no external route. The only useful destination on it
# is the host workspace's egress proxy.
resource "docker_network" "sandbox" {
  count    = data.coder_workspace.me.start_count
  name     = "${local.workspace_name}-sandbox"
  internal = true
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = var.workspace_image
  name     = local.workspace_name
  hostname = data.coder_workspace.me.name

  entrypoint = ["sh", "-c", local.host_agent_init]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",

    # Start the egress proxy without a platform-managed sandbox. Terraform owns
    # the sandbox container below; the agent owns only policy and proxying.
    "CODER_AI_EGRESS_PROXY=true",
    "CODER_AI_SANDBOX_PROXY_ADDRESS=0.0.0.0:13337",
    "CODER_AI_SANDBOX_EGRESS_ENFORCEMENT=forced",
    "CODER_AI_SANDBOX_NAME=${lower(data.coder_workspace.me.name)}",
  ]

  networks_advanced {
    name = docker_network.workspace[0].name
  }

  networks_advanced {
    name    = docker_network.sandbox[0].name
    aliases = ["coder-egress-proxy"]
  }

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
}

resource "docker_container" "sandbox" {
  count    = data.coder_workspace.me.start_count
  image    = var.sandbox_image
  name     = local.sandbox_name
  hostname = "ai-${data.coder_workspace.me.name}"

  # Terraform dependency ordering proves only that the workspace container
  # exists, not that the agent inside it has fetched policy and opened the
  # proxy. Wait for the real listener before starting the bound agent. The
  # bounded wait fails closed instead of launching an agent with no egress path.
  entrypoint = ["bash", "-lc", <<-EOT
    set -euo pipefail
    ready=""
    for _ in $(seq 1 300); do
      if (exec 3<>/dev/tcp/coder-egress-proxy/13337) 2>/dev/null; then
        exec 3>&-
        exec 3<&-
        ready=true
        break
      fi
      sleep 0.2
    done
    if [ "$${ready}" != true ]; then
      echo "egress proxy did not become ready within 60 seconds" >&2
      exit 1
    fi

    printf '%s' '${base64encode(local.ai_agent_init)}' | base64 -d > /tmp/coder-agent-init.sh
    exec sh /tmp/coder-agent-init.sh
  EOT
  ]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.ai.token}",
    "HTTP_PROXY=http://coder-egress-proxy:13337",
    "HTTPS_PROXY=http://coder-egress-proxy:13337",
    "http_proxy=http://coder-egress-proxy:13337",
    "https_proxy=http://coder-egress-proxy:13337",
    "NO_PROXY=localhost,127.0.0.1,::1",
  ]

  # Deliberately no ordinary network. Ignoring HTTP_PROXY yields no route.
  networks_advanced {
    name = docker_network.sandbox[0].name
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
    label = "coder.ai_bound"
    value = "true"
  }

  depends_on = [docker_container.workspace]
}
