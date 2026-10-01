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

resource "coder_devcontainer" "repo" {
  count = 2

  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace/${count.index}"
}

resource "coder_script" "prerequisite" {
  agent_id     = coder_devcontainer.repo[1].subagent_id
  display_name = "Prerequisite"
  script       = "echo prerequisite"
  run_on_start = true
}

resource "coder_script" "dependent" {
  agent_id     = coder_devcontainer.repo[1].subagent_id
  display_name = "Dependent"
  script       = "echo dependent"
  run_on_start = true
}
