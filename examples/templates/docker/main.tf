terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.3"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.16.0"
    }
  }
}

# Admin parameters

# Comment this out if you are specifying a different docker
# host on the "docker" provider below.
variable "step1_docker_host_warning" {
  description = <<-EOF
  This template will use the Docker socket present on
  the Coder host, which is not necessarily your local machine.

  You can specify a different host in the template file and
  surpress this warning.
  EOF
  validation {
    condition     = contains(["Continue using /var/run/docker.sock on the Coder host"], var.step1_docker_host_warning)
    error_message = "Cancelling template create."
  }

  sensitive = true
}
variable "step2_arch" {
  description = <<-EOF
  arch: What architecture is your Docker host on?

  note: codercom/enterprise-* images are only built for amd64
  EOF

  validation {
    condition     = contains(["amd64", "arm64", "armv7"], var.step2_arch)
    error_message = "Value must be amd64, arm64, or armv7."
  }
  sensitive = true
}
variable "step3_OS" {
  description = <<-EOF
  What operating system is your Coder host on?
  EOF

  validation {
    condition     = contains(["MacOS", "Windows", "Linux"], var.step3_OS)
    error_message = "Value must be MacOS, Windows, or Linux."
  }
  sensitive = true
}

provider "docker" {
  host = var.step3_OS == "Windows" ? "npipe:////.//pipe//docker_engine" : "unix:///var/run/docker.sock"
}

provider "coder" {
}

data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  arch           = var.step2_arch
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh
    code-server --auth none --port 13337
    EOF

  # These environment variables allow you to make Git commits right away after creating a
  # workspace. Note that they take precedence over configuration defined in ~/.gitconfig!
  # You can remove this block if you'd prefer to configure Git manually or using
  # dotfiles. (see docs/dotfiles.md)
  env = {
    GIT_AUTHOR_NAME = "${data.coder_workspace.me.owner}"
    GIT_COMMITTER_NAME = "${data.coder_workspace.me.owner}"
    GIT_AUTHOR_EMAIL = "${data.coder_workspace.me.owner_email}"
    GIT_COMMITTER_EMAIL = "${data.coder_workspace.me.owner_email}"
  }
}

resource "coder_app" "code-server" {
  agent_id = coder_agent.main.id
  name     = "code-server"
  url      = "http://localhost:13337/?folder=/home/coder"
  icon     = "/icon/code.svg"
}


variable "docker_image" {
  description = "Which Docker image would you like to use for your workspace?"
  # The codercom/enterprise-* images are only built for amd64
  default = "codercom/enterprise-base:ubuntu"
  validation {
    condition = contains(["codercom/enterprise-base:ubuntu", "codercom/enterprise-node:ubuntu",
    "codercom/enterprise-intellij:ubuntu", "codercom/enterprise-golang:ubuntu"], var.docker_image)
    error_message = "Invalid Docker image!"
  }

}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-home"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = var.docker_image
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
    ${replace(coder_agent.main.init_script, "localhost", "host.docker.internal")}
    EOT
  ]
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
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
