# Note: this example demonstrates the use of
# dotfiles with Coder templates.

# The Docker aspect of the template only works
# with macOS/Linux amd64 systems. See the full
# Docker example for details

terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.15"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.20.2"
    }
  }
}

data "coder_provisioner" "me" {
}

provider "docker" {
}

data "coder_workspace" "me" {
}

variable "docker_image" {
  default = "codercom/enterprise-base:ubuntu"
}

variable "dotfiles_uri" {
  description = <<-EOF
  Dotfiles repo URI (optional)

  see https://dotfiles.github.io
  EOF
  default     = ""
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = var.dotfiles_uri != "" ? "coder dotfiles -y ${var.dotfiles_uri}" : null
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}-root"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = var.docker_image
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  dns  = ["1.1.1.1"]
  # Refer to Docker host when Coder is on localhost
  command = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
}

resource "coder_metadata" "container_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = docker_container.workspace[0].id

  item {
    key   = "image"
    value = var.docker_image
  }
}
