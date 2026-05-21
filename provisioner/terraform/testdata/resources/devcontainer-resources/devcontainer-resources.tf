terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_devcontainer" "dev" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace"
}

resource "coder_app" "devcontainer-app" {
  agent_id = coder_devcontainer.dev.subagent_id
  slug     = "devcontainer-app"
}

resource "coder_script" "devcontainer-script" {
  agent_id     = coder_devcontainer.dev.subagent_id
  display_name = "Devcontainer Script"
  script       = "echo devcontainer"
  run_on_start = true
}

resource "coder_env" "devcontainer-env" {
  agent_id = coder_devcontainer.dev.subagent_id
  name     = "DEVCONTAINER_ENV"
  value    = "devcontainer-value"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}
