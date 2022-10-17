terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      # future versions of coder/coder have built-in regex testing for valid
      # app names, so we can't use a version after this.
      version = "0.5.3"
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

resource "coder_app" "invalid_app_name" {
  agent_id = coder_agent.dev.id
}
