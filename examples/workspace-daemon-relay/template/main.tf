# A workspace in a Docker container that hands its owner's session token to
# what runs in it, the way templates for coding agents do. Nothing here knows
# about the workspace relay: where the control plane has it turned on, the
# workspace's daemon replaces CODER_URL with its own local address.
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

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

# What a template sets today: the control plane's own address, and the
# owner's session token.
resource "coder_env" "coder_url" {
  agent_id = coder_agent.main.id
  name     = "CODER_URL"
  value    = data.coder_workspace.me.access_url
}

resource "coder_env" "coder_session_token" {
  agent_id = coder_agent.main.id
  name     = "CODER_SESSION_TOKEN"
  value    = data.coder_workspace_owner.me.session_token
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-base:ubuntu"
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  # Use the Docker gateway when the access URL is on the loopback interface.
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}
