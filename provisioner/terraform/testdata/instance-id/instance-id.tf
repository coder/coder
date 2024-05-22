terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  auth = "google-instance-identity"
}

resource "null_resource" "main" {
  depends_on = [
    coder_agent.main
  ]
}

resource "coder_agent_instance" "main" {
  agent_id    = coder_agent.main.id
  instance_id = "example"
}
