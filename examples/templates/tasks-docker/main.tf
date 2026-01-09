terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

provider "docker" {}

data "coder_task" "me" {}
data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

variable "claude_api_key" {
  description = "API key for Claude"
  type        = string
  sensitive   = true
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e
    # Prepare user home with default files on first start.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi
    mkdir -p /home/coder/projects
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
    ANTHROPIC_API_KEY   = var.claude_api_key
  }
}

module "code-server" {
  count  = data.coder_workspace.me.start_count
  folder = "/home/coder/projects"
  source = "registry.coder.com/coder/code-server/coder"

  settings = {
    "workbench.colorTheme" : "Default Dark Modern"
  }

  # This ensures that the latest non-breaking version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = "~> 1.0"

  agent_id = coder_agent.main.id
  order    = 1
}

# Keeping statless for experimentation
# resource "docker_volume" "home_volume" {
# name = "coder-${data.coder_workspace.me.id}-home"
# Protect the volume from being deleted due to changes in attributes.
# lifecycle {
# ignore_changes = all
# }
# Add labels in Docker to keep track of orphan resources.
# labels {
# label = "coder.owner"
# value = data.coder_workspace_owner.me.name
# }
# labels {
# label = "coder.owner_id"
# value = data.coder_workspace_owner.me.id
# }
# labels {
# label = "coder.workspace_id"
# value = data.coder_workspace.me.id
# }
# This field becomes outdated if the workspace is renamed but can
# be useful for debugging or cleaning out dangling volumes.
# labels {
# label = "coder.workspace_name_at_creation"
# value = data.coder_workspace.me.name
# }
# }

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = "codercom/enterprise-node:ubuntu"
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname   = data.coder_workspace.me.name
  user       = "coder"
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
    "CODER_AGENT_SOCKET_SERVER_ENABLED=true",
    "CODER_TASK_ID=${data.coder_task.me.id}",
    "CODER_TASK_INITIAL_PROMPT=${data.coder_task.me.prompt}",
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  # Keeping stateless for experimentation
  # volumes {
  # container_path = "/home/coder"
  # volume_name    = docker_volume.home_volume.name
  # read_only      = false
  # }

  # Add labels in Docker to keep track of orphan resources.
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

resource "coder_script" "claude-code-acp" {
  count        = data.coder_workspace.me.start_count
  agent_id     = coder_agent.main.id
  display_name = "Sets up the ACP client for Claude Code"
  run_on_start = true
  script       = <<EOT
    #!/bin/bash
    set -euo pipefail
    set -x

    trap 'coder exp sync complete script-claude-code-acp' EXIT
    #coder exp sync want script-claude-code-acp module-claude-code
    coder exp sync start script-claude-code-acp

    sudo apt-get update
    sudo apt-get install -y screen
    # For some reason, this fails with a Killed signal
    # curl -fsSL https://claude.ai/install.sh > /tmp/install-claude.sh
    # sudo bash /tmp/install-claude.sh
    sudo npm install -g @anthropic-ai/claude-code
    sudo npm install -g @zed-industries/claude-code-acp
    screen -dmS claude-code-acp /bin/sh -c 'coder exp acp stdio-ws --verbose -- claude-code-acp'
  EOT
}

resource "coder_app" "claude-code-acp" {
  count        = data.coder_workspace.me.start_count
  agent_id     = coder_agent.main.id
  slug         = "claude-code-acp"
  display_name = "Stdio WebSocket for Claude Code ACP"
  url          = "http://localhost:8080" # Default port for stdio-ws is 8080
  share        = "authenticated"
  open_in      = "tab"
  order        = 0
}

resource "coder_ai_task" "task" {
  count  = data.coder_workspace.me.start_count
  app_id = coder_app.claude-code-acp[count.index].id
}
