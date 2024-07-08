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

variable "go_image" {
  description = "Go SDK image reference"
  type        = string
}

variable "java_image" {
  description = "Java image reference."
  type        = string
}

locals {
  username = data.coder_workspace_owner.me.name

  images = {
    "go"   = var.go_image,
    "java" = var.java_image,
  }
}

data "coder_provisioner" "me" {
}

data "coder_workspace" "me" {
}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "container_image" {
  name         = "container_image"
  display_name = "Workspace container image"
  description  = "Select the container image for your development environment."
  default      = "java"
  mutable      = true

  dynamic "option" {
    for_each = [for k in keys(local.images) : { name = k, value = lower(k) }]
    content {
      name  = option.value.name
      value = option.value.value
    }
  }
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<EOF
    #!/bin/sh
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --version 4.8.3
    code-server --auth none --port 13337
    EOF

  env = {
    GIT_AUTHOR_NAME     = "${data.coder_workspace_owner.me.name}"
    GIT_COMMITTER_NAME  = "${data.coder_workspace_owner.me.name}"
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
  }
}

resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  url          = "http://localhost:13337/?folder=/home/${local.username}"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
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
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "coder_metadata" "home_info" {
  resource_id = docker_volume.home_volume.id

  item {
    key   = "size"
    value = "5 GiB"
  }
}

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = local.images[data.coder_parameter.container_image.value]
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_PARAMETER_CONTAINER_IMAGE=${local.images[data.coder_parameter.container_image.value]}"
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/${local.username}"
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
