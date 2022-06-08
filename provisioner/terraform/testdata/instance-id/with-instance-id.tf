terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  auth = "google-instance-identity"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev
  ]
}

resource "coder_agent_instance" "dev" {
  agent_id    = coder_agent.dev.id
  instance_id = "example"
}
