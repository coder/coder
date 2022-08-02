terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.18.0"
    }
  }
}

# Admin parameters

provider "docker" {
  host = "unix:///var/run/dogfood-docker.sock"
}

provider "coder" {
}

data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  arch           = "amd64"
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh
    set -x
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh
    code-server --auth none --port 13337 &
    sudo service docker start
    if [ -f ~/personalize ]; then ~/personalize 2>&1 | tee  ~/.personalize.log; fi
    EOF
}

resource "coder_app" "code-server" {
  agent_id = coder_agent.main.id
  name     = "code-server"
  url      = "http://localhost:13337/?folder=/home/coder"
  icon     = "/icon/code.svg"
}


resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-home"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "gcr.io/coder-dogfood/master/coder-dev-ubuntu:latest"
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = lower(data.coder_workspace.me.name)
  dns      = ["1.1.1.1"]
  # Use the docker gateway if the access URL is 127.0.0.1
  command = [
    "sh", "-c",
    <<EOT
    trap '[ $? -ne 0 ] && echo === Agent script exited with non-zero code. Sleeping infinitely to preserve logs... && sleep infinity' EXIT
    ${replace(coder_agent.dev.init_script, "localhost", "host.docker.internal")}
    EOT
  ]
  # CPU limits are unnecessary since Docker will load balance automatically
  memory  = 32768
  runtime = "sysbox-runc"
  env     = ["CODER_AGENT_TOKEN=${coder_agent.dev.token}"]
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
