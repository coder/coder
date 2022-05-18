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

provider "coder" {
  # The below assumes your Coder deployment is running in docker-compose.
  # If this is not the case, either comment or edit the below.
  url = "http://host.docker.internal:7080"
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
    condition     = contains(["codercom/enterprise-base:ubuntu", "codercom/enterprise-node:ubuntu", "codercom/enterprise-intellij:ubuntu"], var.docker_image)
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
  env     = ["CODER_AGENT_TOKEN=${coder_agent.dev.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.coder_volume.name
    read_only      = false
  }
}
