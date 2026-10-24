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

# Harbor builds the task image locally and bakes the `coder` binary into it,
# so the container runs the agent directly instead of the curl-based
# init_script (task base images often lack curl and wget).

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "image" {
  name         = "image"
  display_name = "Task image"
  description  = "Image built by the Harbor coder environment provider."
  type         = "string"
  mutable      = true
}

data "coder_parameter" "workdir" {
  name         = "workdir"
  display_name = "Working directory"
  type         = "string"
  default      = "/"
  mutable      = true
}

data "coder_parameter" "cpus" {
  name    = "cpus"
  type    = "number"
  default = 1
  mutable = true
}

data "coder_parameter" "memory_mb" {
  name    = "memory_mb"
  type    = "number"
  default = 2048
  mutable = true
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
}

resource "docker_container" "task" {
  count      = data.coder_workspace.me.start_count
  image      = data.coder_parameter.image.value
  name       = "harbor-${data.coder_workspace.me.id}"
  hostname   = data.coder_workspace.me.name
  user       = "root"
  cpu_shares = data.coder_parameter.cpus.value * 1024
  memory     = data.coder_parameter.memory_mb.value

  entrypoint = ["/usr/local/bin/coder", "agent"]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    # The access URL is localhost on CI runners; the container reaches the
    # host through the docker gateway alias instead.
    "CODER_AGENT_URL=${replace(data.coder_workspace.me.access_url, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")}",
    "CODER_AGENT_LOG_DIR=/tmp",
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}
