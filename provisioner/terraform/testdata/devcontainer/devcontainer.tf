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

resource "coder_devcontainer" "dev1" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace1"
}

resource "coder_devcontainer" "dev2" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace2"
  config_path      = "/workspace2/.devcontainer/devcontainer.json"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}
