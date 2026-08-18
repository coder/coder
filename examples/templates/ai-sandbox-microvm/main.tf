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
  description = <<-EOT
    Image for the human workspace container. It must contain coder-sandbox,
    bash, and a static Linux amd64 coder binary on PATH.
  EOT
  type        = string
  default     = "codercom/enterprise-base:ubuntu"
}

variable "sandbox_image" {
  description = "OCI image booted inside the coder/sandbox microVM."
  type        = string
  default     = "ubuntu:24.04"
}

variable "sandbox_memory_mib" {
  description = "Guest memory for the coder/sandbox microVM, in MiB."
  type        = number
  default     = 1024
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
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
  staging_dir    = "/tmp/coder-ai-microvm"
  policy_file    = "/home/coder/.config/coder-sandbox/coder-ai/runtime-network.yaml"

  agent_init = replace(
    coder_agent.main.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )

  # These variables must be present in the agent process environment. The
  # controller reads them while the agent starts, before startup scripts run.
  agent_process_env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AI_SANDBOX_CREATE_SCRIPT=${local.staging_dir}/sandbox-create.sh",
    "CODER_AI_SANDBOX_DESTROY_SCRIPT=${local.staging_dir}/sandbox-destroy.sh",
    "CODER_AI_SANDBOX_POLICY_FILE=${local.policy_file}",
    "CODER_AI_SANDBOX_POLICY_RELOAD_SCRIPT=${local.staging_dir}/sandbox-reload-policy.sh",
    "CODER_AI_SANDBOX_NAME=microvm",
    "CODER_AI_SANDBOX_IMAGE=${var.sandbox_image}",
    "CODER_AI_SANDBOX_MEMORY_MIB=${var.sandbox_memory_mib}",
  ]
}

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

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }
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

  # Stage scripts before starting the agent. A bind mount from path.module does
  # not work because Docker resolves host paths on the daemon host.
  entrypoint = ["sh", "-c", <<-EOT
    set -eu
    mkdir -p ${local.staging_dir}
    echo '${base64encode(file("${path.module}/scripts/sandbox-create.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-create.sh
    echo '${base64encode(file("${path.module}/scripts/sandbox-destroy.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-destroy.sh
    echo '${base64encode(file("${path.module}/scripts/sandbox-reload-policy.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-reload-policy.sh
    echo '${base64encode(local.agent_init)}' | base64 -d > ${local.staging_dir}/init.sh
    chmod +x ${local.staging_dir}/sandbox-create.sh ${local.staging_dir}/sandbox-destroy.sh ${local.staging_dir}/sandbox-reload-policy.sh
    exec sh ${local.staging_dir}/init.sh
  EOT
  ]

  env       = local.agent_process_env
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

  # The whole home directory is persistent. coder/sandbox keeps daemon state,
  # downloaded runtime artifacts, and image caches below this mount.
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
