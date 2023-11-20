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

provider "docker" {}

provider "coder" {}

data "coder_workspace" "me" {}

data "coder_parameter" "image" {
  name = "image"
  type = "string"
  icon = "${data.coder_workspace.me.access_url}/icon/docker.png"
  option {
    value = "codercom/enterprise-node:ubuntu"
    name  = "node"
  }
  option {
    value = "codercom/enterprise-golang:ubuntu"
    name  = "golang"
  }
  option {
    value = "codercom/enterprise-java:ubuntu"
    name  = "java"
  }
  option {
    value = "codercom/enterprise-base:ubuntu"
    name  = "base"
  }
}
data "coder_parameter" "repo" {
  name    = "repo"
  type    = "string"
  default = "eric/react-demo.git"
  icon    = "https://git-scm.com/images/logos/downloads/Git-Icon-1788C.png"
}

resource "coder_agent" "dev" {
  arch           = "amd64"
  os             = "linux"
  startup_script = <<EOT
#!/bin/bash

# Start Docker
sudo dockerd &

# clone repo
mkdir -p ~/.ssh
ssh-keyscan -t ed25519 github.com >> ~/.ssh/known_hosts
git clone ${data.coder_parameter.repo.value}

# install code-server
curl -fsSL https://code-server.dev/install.sh | sh
code-server --auth none --port 13337 &

  EOT
}

resource "coder_app" "code-server" {
  agent_id     = coder_agent.dev.id
  slug         = "code-server"
  display_name = "VS Code"
  url          = "http://localhost:13337/?folder=/home/coder"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 15
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "ghcr.io/coder/envbox:latest"
  # Uses lower() to avoid Docker restriction on container names.
  name     = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  hostname = lower(data.coder_workspace.me.name)
  dns      = ["1.1.1.1"]

  # Use the docker gateway if the access URL is 127.0.0.1
  #entrypoint = ["sh", "-c", replace(coder_agent.dev.init_script, "127.0.0.1", "host.docker.internal")]

  # Use the docker gateway if the access URL is 127.0.0.1
  command = [
    "sh", "-c", "/envbox docker",
    <<EOT
    trap '[ $? -ne 0 ] && echo === Agent script exited with non-zero code. Sleeping infinitely to preserve #logs... && sleep infinity' EXIT
    ${replace(coder_agent.dev.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")}
    EOT
  ]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
    "CODER_AGENT_URL=${data.coder_workspace.me.access_url}",
    "CODER_INNER_IMAGE=${data.coder_parameter.image.value}",
    "CODER_INNER_USERNAME=coder",
    "CODER_BOOTSTRAP_SCRIPT=${coder_agent.dev.init_script}",
    "CODER_MOUNTS=/home/coder:/home/coder",
    "CODER_ADD_FUSE=false",
    "CODER_INNER_HOSTNAME=${data.coder_workspace.me.name}",
    "CODER_ADD_TUN=false",
    "CODER_CPUS=1",
    "CODER_MEMORY=2"
  ]

  privileged = true

  volumes {
    volume_name    = docker_volume.coder_volume.name
    container_path = "/home/coder/"
    read_only      = false
  }

  volumes {
    volume_name    = "sysbox"
    container_path = "/var/lib/sysbox"
  }

  volumes {
    host_path      = "/usr/src"
    container_path = "/usr/src"
    read_only      = true
  }

  volumes {
    host_path      = "/lib/modules"
    container_path = "/lib/modules"
    read_only      = true
  }

  mounts {
    target = "/var/lib/coder/docker"
    source = docker_volume.coder_volume.name
    type   = "volume"
  }

  mounts {
    target = "/var/lib/coder/containers"
    source = docker_volume.coder_volume.name
    type   = "volume"
  }

  mounts {
    target = "/var/lib/containers"
    source = docker_volume.coder_volume.name
    type   = "volume"
  }
  mounts {
    target = "/var/lib/docker"
    source = docker_volume.coder_volume.name
    type   = "volume"
  }

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}

resource "docker_volume" "coder_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
}

resource "docker_volume" "sysbox" {
  name = "sysbox"
}
