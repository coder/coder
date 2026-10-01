terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

variable "name" {
  type = string
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_devcontainer" "repo" {
  agent_id         = coder_agent.main.id
  workspace_folder = "/workspace/${var.name}"
}

output "agent_id" {
  value = coder_agent.main.id
}

output "parent_agent_id" {
  value = coder_devcontainer.repo.agent_id
}
