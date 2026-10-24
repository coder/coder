terraform {
  required_providers {
    coder  = { source = "coder/coder" }
    docker = { source = "kreuzwerker/docker" }
  }
}

provider "docker" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}

resource "coder_agent" "runner" {
  arch                    = data.coder_provisioner.me.arch
  os                      = "linux"
  startup_script_behavior = "non-blocking"
  startup_script          = <<-EOT
    #!/bin/bash
    set -euo pipefail
    cd /home/runner
    exec ./run.sh
  EOT
}

# Maintain this image version as GitHub releases runner updates. No workspace
# volume or host Docker socket is shared with workflow jobs.
resource "docker_image" "runner" {
  name         = "ghcr.io/actions/actions-runner:2.338.1"
  keep_locally = true
}

resource "docker_container" "runner" {
  count      = data.coder_workspace.me.start_count
  name       = "coder-gha-${data.coder_workspace.me.id}"
  image      = docker_image.runner.image_id
  entrypoint = ["sh", "-c", replace(coder_agent.runner.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.runner.token}"]
  memory     = 2048
  cpu_shares = 512
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
}
