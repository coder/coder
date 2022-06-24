terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.16.0"
    }
  }
}

variable "docker_host" {
  description = "Specify location of Docker socket (check `docker context ls` if you're not sure)"
  sensitive   = true
}

variable "docker_arch" {
  description = "Specify architecture of docker host (amd64, arm64, or armv7)"
  validation {
    condition     = contains(["amd64", "arm64", "armv7"], var.docker_arch)
    error_message = "Value must be amd64, arm64, or armv7."
  }
  sensitive = true
}

provider "coder" {
}

provider "docker" {
  host = var.docker_host
}

data "coder_workspace" "me" {
}

resource "coder_agent" "dev" {
  arch           = var.docker_arch
  os             = "linux"
  startup_script = "code-server --auth none"
}

resource "coder_app" "code-server" {
  agent_id = coder_agent.dev.id
  url      = "http://localhost:8080/?folder=/home/coder"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/code-server:latest"
  # Uses lower() to avoid Docker restriction on container names.
  name     = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  hostname = lower(data.coder_workspace.me.name)
  dns      = ["1.1.1.1"]
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", replace(coder_agent.dev.init_script, "127.0.0.1", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.dev.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}
