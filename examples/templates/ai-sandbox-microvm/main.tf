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

variable "docker_socket" {
  description = "(Optional) Docker socket URI used by the provisioner."
  type        = string
  default     = ""
}

variable "workspace_image" {
  description = "Image for the workspace container that runs the host Coder agent."
  type        = string
  default     = "codercom/enterprise-base:ubuntu"
}

variable "sandbox_image" {
  description = "OCI image booted inside the embedded microVM."
  type        = string
  default     = "ubuntu:24.04"
}

variable "sandbox_memory_mib" {
  description = "Guest memory for the embedded microVM, in MiB."
  type        = number
  default     = 1024
}

variable "mcp_server_slugs" {
  description = <<-EOT
    Slugs of MCP servers (Admin settings, AI, MCP Servers) to register in
    Claude Code inside the sandbox. Each connects through the Coder MCP
    gateway using the AI agent identity's session token.
  EOT
  type        = list(string)
  default     = ["github"]
}

variable "sandbox_cpus" {
  description = "Virtual CPU count for the embedded microVM."
  type        = number
  default     = 1
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# Using this data source opts the workspace into an AI agent identity:
# coderd detects it at template import and mints a scoped session token
# at build time, sponsored by the workspace owner.
data "coder_workspace_ai_agent" "me" {}

data "coder_parameter" "kvm_gid" {
  name         = "kvm_gid"
  display_name = "KVM device group ID"
  description  = "Numeric group ID of /dev/kvm on the Docker host. Run: stat -c %g /dev/kvm"
  type         = "string"
  default      = "108"
  mutable      = true
  order        = 1
}

locals {
  workspace_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  host_agent_init = replace(
    coder_agent.main.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"

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

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }
}

resource "coder_agent" "ai" {
  arch = "amd64"
  os   = "linux"

  ai_bound           = true
  egress_enforcement = "forced"

  env = {
    # Route Claude Code's model traffic through the Coder AI gateway,
    # authenticated as the AI agent identity. No provider API key ever
    # enters the sandbox; the gateway injects centralized credentials
    # server-side and meters usage per identity.
    ANTHROPIC_BASE_URL   = "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/anthropic"
    ANTHROPIC_AUTH_TOKEN = data.coder_workspace_ai_agent.me.session_token
  }

  # Installs Claude Code inside the microVM guest. The installer runs
  # through the sandbox egress proxy, so the AI egress policy must allow
  # the download hosts (claude.ai and storage.googleapis.com); the README
  # lists the rules. Failure leaves the agent usable; the app below just
  # reports the missing binary.
  startup_script = <<-EOT
    set -eu
    if command -v claude >/dev/null 2>&1; then
      echo "Claude Code already installed: $(claude --version)"
    else
      # The stock ubuntu guest ships without curl. Package installs work
      # because apt honors the proxy environment and the sandbox host
      # process holds CAP_CHOWN.
      if ! command -v curl >/dev/null 2>&1; then
        echo "Installing curl..."
        apt-get update -qq && apt-get install -y -qq curl ca-certificates
      fi
      echo "Installing Claude Code..."
      curl -fsSL https://claude.ai/install.sh | bash
      # The installer targets ~/.local/bin, which is not on PATH for
      # login shells in the minimal guest image.
      if [ -x "$HOME/.local/bin/claude" ]; then
        ln -sf "$HOME/.local/bin/claude" /usr/local/bin/claude || true
      fi
      claude --version
    fi

    # Register configured MCP servers through the Coder MCP gateway,
    # ALWAYS, not only on fresh installs: the guest rootfs is rebuilt
    # from the OCI image on every boot, so client config does not
    # persist. MCP is client-configured; the gateway endpoint exists
    # regardless, but Claude Code only queries servers in its own
    # config. Registration failures must not fail the boot.
    %{~for slug in var.mcp_server_slugs~}
    claude mcp add --transport http --scope user \
      --header "Authorization: Bearer $CODER_SESSION_TOKEN" \
      "${slug}" "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/mcp/${slug}" ||
      echo "warning: failed to register MCP server ${slug}"
    %{~endfor~}
    claude mcp list || true
  EOT
}

resource "coder_app" "claude_code" {
  agent_id     = coder_agent.ai.id
  slug         = "claude-code"
  display_name = "Claude Code"
  icon         = "/icon/claude.svg"
  command      = "claude"
}

resource "coder_script" "sandbox" {
  agent_id           = coder_agent.main.id
  display_name       = "Start AI microVM sandbox"
  run_on_start       = true
  start_blocks_login = false
  script             = <<-EOT
    #!/bin/sh
    set -eu

    log_file=/tmp/coder-agent-sandbox.log
    pid_file=/tmp/coder-agent-sandbox.pid

    if [ -s "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
      echo "coder agent sandbox is already running with PID $(cat "$pid_file")" >>"$log_file"
      exit 0
    fi

    coder_bin="$(readlink -f /proc/$PPID/exe 2>/dev/null || true)"
    case "$coder_bin" in
      *coder*) ;;
      *) coder_bin="$(command -v coder || true)" ;;
    esac
    if [ ! -x "$coder_bin" ]; then
      echo "cannot locate coder agent binary" >&2
      exit 1
    fi

    # The sandbox host process serves the guest's virtio-fs filesystem, so
    # guest chown calls execute on the host with this process's privileges.
    # Without CAP_CHOWN, guest package managers fail with ownership errors.
    # Run as root when passwordless sudo is available; the microVM remains
    # the security boundary. Cache and state stay under the persistent home
    # volume so first-boot downloads are not repeated.
    run_as=""
    if [ "$(id -u)" != "0" ] && command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
      run_as="sudo -E"
    else
      echo "warning: no root or passwordless sudo; guest chown will fail (CAP_CHOWN)" >>"$log_file"
    fi

    echo "starting coder agent sandbox at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$log_file"
    nohup $run_as "$coder_bin" agent sandbox \
      --cache-dir "$HOME/.config/coder-ai/microvm/cache" \
      --state-dir "$HOME/.config/coder-ai/microvm/state" \
      </dev/null >>"$log_file" 2>&1 &
    sandbox_pid=$!
    echo "$sandbox_pid" >"$pid_file"
    echo "started coder agent sandbox with PID $sandbox_pid; logs: $log_file"
  EOT
}

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
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = var.workspace_image
  name     = local.workspace_name
  hostname = data.coder_workspace.me.name

  entrypoint = ["sh", "-c", local.host_agent_init]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_SANDBOX_AGENT_TOKEN=${coder_agent.ai.token}",
    # Scoped AI identity token, delivered to the guest agent as
    # CODER_SESSION_TOKEN so sandboxed tools can reach the AI gateway,
    # including its MCP endpoint. Empty when the deployment does not
    # provision an AI agent identity.
    "CODER_SANDBOX_SESSION_TOKEN=${data.coder_workspace_ai_agent.me.session_token}",
    "CODER_SANDBOX_IMAGE=${var.sandbox_image}",
    "CODER_SANDBOX_MEMORY_MIB=${var.sandbox_memory_mib}",
    "CODER_SANDBOX_CPUS=${var.sandbox_cpus}",
  ]

  group_add = [data.coder_parameter.kvm_gid.value]

  devices {
    host_path      = "/dev/kvm"
    container_path = "/dev/kvm"
    permissions    = "rwm"
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

  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
