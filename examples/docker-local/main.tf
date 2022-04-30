terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.3.4"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.16.0"
    }
  }
}

provider "docker" {
  host = "unix:///var/run/docker.sock"
}

data "coder_workspace" "me" {
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
}

variable "docker_image" {
  description = "What docker image would you like to use for your workspace?"
  default     = "codercom/enterprise-base:ubuntu"
  validation {
    condition     = contains(["codercom/enterprise-base:ubuntu", "codercom/enterprise-node:ubuntu", "codercom/enterprise-java:ubuntu"], var.docker_image)
    error_message = "Invalid Docker Image!"
  }
}

resource "docker_volume" "coder_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
}

resource "docker_container" "workspace" {
  count   = data.coder_workspace.me.start_count
  image   = var.docker_image
  name    = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  dns     = ["1.1.1.1"]
  command = ["sh", "-c", coder_agent.dev.init_script]
  env     = ["CODER_TOKEN=${coder_agent.dev.token}"]
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.coder_volume.name
    read_only      = false
  }
}
