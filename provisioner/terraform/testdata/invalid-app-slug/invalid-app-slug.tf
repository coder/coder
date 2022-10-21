terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.0"
    }
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev
  ]
}

resource "coder_app" "invalid-app-slug" {
  agent_id = coder_agent.dev.id
  slug     = "$$$"
}
