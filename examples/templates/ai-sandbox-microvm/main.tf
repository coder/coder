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

    echo "starting coder agent sandbox at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$log_file"
    nohup "$coder_bin" agent sandbox </dev/null >>"$log_file" 2>&1 &
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
