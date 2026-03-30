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

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

variable "docker_socket" {
  default     = ""
  description = "Docker socket URI"
  type        = string
  sensitive   = true
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : (
    data.coder_provisioner.me.operating_system == "windows" ?
    "npipe:////.//pipe//docker_engine" :
    "unix:///var/run/docker.sock"
  )
}

locals {
  # Terraform treats repeated references to the hidden chat agent as awkward,
  # so capture the generated bootstrap values once and reuse them below.
  chat_agent_init  = coder_agent.dev-coderd-chat.init_script
  chat_agent_token = coder_agent.dev-coderd-chat.token
}

resource "coder_agent" "dev" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  startup_script = <<-EOT
    set -e

    # Seed the home directory and install base packages only on first boot.
    # The shared Docker volume persists across restarts, so repeating apt work
    # would slow every start without adding value.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      sudo apt-get update
      sudo apt-get install -y curl git ca-certificates
      touch ~/.init_done
    fi
  EOT

  # Pre-populate Git identity so the visible development agent is ready for
  # commits without requiring each workspace owner to reconfigure Git.
  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }

  # Surface a few standard stats on the visible agent so the example still
  # feels like a normal Docker workspace from the dashboard.
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

  metadata {
    display_name = "Load Average (Host)"
    key          = "6_load_host"
    # Show load normalized by CPU count so the value stays useful on machines
    # with different core counts.
    script   = <<-EOT
      echo "$(cat /proc/loadavg | awk '{ print $1 }') $(nproc)" |
        awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }

}

resource "coder_agent" "dev-coderd-chat" {
  arch  = data.coder_provisioner.me.arch
  os    = "linux"
  order = 99

  # Mirror the owner Git identity in the hidden chat agent because it shares
  # the same persistent home directory and may create commits there.
  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"

  # Keep the shared home volume stable so restarts do not discard user data or
  # Terraform-drifted metadata.
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
    label = "coder.workspace_id_at_creation"
    value = data.coder_workspace.me.id
  }
}

resource "docker_image" "boundary_sandbox" {
  name = "coder-boundary-sandbox-${data.coder_workspace.me.id}"

  build {
    context    = path.module
    dockerfile = "Dockerfile.boundary"
  }

  # Rebuild when the image, wrapper, or policy changes so the hidden agent does
  # not keep an outdated network policy.
  triggers = {
    file_sha1 = sha1(join("", [
      file("${path.module}/Dockerfile.boundary"),
      file("${path.module}/boundary-agent.sh"),
      file("${path.module}/boundary-config.yaml"),
    ]))
  }
}

module "code-server" {
  source   = "dev.registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.dev.id
}

resource "docker_container" "dev" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"

  # Keep the visible development container name aligned with other Docker
  # template examples so orphaned resources are easy to identify.
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name

  # Rewrite loopback URLs so the generated init script can still reach a Coder
  # server bound on the Docker host.
  entrypoint = [
    "sh",
    "-c",
    replace(coder_agent.dev.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal"),
  ]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
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

resource "docker_container" "chat" {
  count = data.coder_workspace.me.start_count
  image = docker_image.boundary_sandbox.image_id

  # Name the hidden container predictably so it is easy to correlate with the
  # visible workspace container during debugging.
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}-chat"
  hostname = "${data.coder_workspace.me.name}-chat"

  # nsjail needs network namespace privileges, but the container should still
  # drop every capability that the sandbox itself does not require.
  capabilities {
    add  = ["NET_ADMIN"]
    drop = ["ALL"]
  }

  # Docker's default seccomp profile blocks clone patterns that nsjail needs to
  # create the boundary namespace.
  security_opts = ["seccomp=unconfined"]

  # The chat agent runs behind a boundary wrapper. Base64 encoding preserves the
  # generated init script without fighting Terraform or shell quoting rules.
  entrypoint = [
    "sh",
    "-c",
    "echo ${base64encode(replace(local.chat_agent_init, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal"))} | base64 -d > /tmp/coder-init.sh && chmod +x /tmp/coder-init.sh && exec boundary-agent sh /tmp/coder-init.sh",
  ]

  env = [
    "CODER_AGENT_TOKEN=${local.chat_agent_token}",
    "CODER_URL=${replace(data.coder_workspace.me.access_url, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")}",
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
