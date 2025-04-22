terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.3.0-pre2"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  # Defaulting to null if the variable is an empty string lets us have an optional variable without having to set our own default
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_workspace_preset" "goland" {
  name = "I Like GoLand"
  parameters = {
    "jetbrains_ide" = "GO"
  }
  prebuilds {
    instances = 2
  }
}

data "coder_workspace_preset" "python" {
  name = "Some Like PyCharm"
  parameters = {
    "jetbrains_ide" = "PY"
  }
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e

    # Prepare user home with default files on first start!
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    if [[ "${data.coder_workspace.me.prebuild_count}" -eq 1 ]]; then
      touch ~/.prebuild_note
    fi
  EOT

  env = {
    OWNER_EMAIL = data.coder_workspace_owner.me.email
  }

  # The following metadata blocks are optional. They are used to display
  # information about your workspace in the dashboard. You can remove them
  # if you don't want to display any information.
  # For basic resources, you can use the `coder stat` command.
  # If you need more control, you can write your own script.
  metadata {
    display_name = "Was Prebuild"
    key          = "prebuild"
    script       = "[[ -e ~/.prebuild_note ]] && echo 'Yes' || echo 'No'"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Owner"
    key          = "owner"
    script       = "echo $OWNER_EMAIL"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Hostname"
    key          = "hostname"
    script       = "hostname"
    interval     = 10
    timeout      = 1
  }
}

# See https://registry.coder.com/modules/jetbrains-gateway
module "jetbrains_gateway" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/jetbrains-gateway/coder"

  # JetBrains IDEs to make available for the user to select
  jetbrains_ides = ["IU", "PY", "WS", "PS", "RD", "CL", "GO", "RM"]
  default        = "IU"

  # Default folder to open when starting a JetBrains IDE
  folder = "/home/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id   = coder_agent.main.id
  agent_name = "main"
  order      = 2
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  lifecycle {
    ignore_changes = all
  }

  network_mode = "host"

  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = [
    "sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
  ]
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
}
