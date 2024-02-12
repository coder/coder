terraform {
  required_providers {
    coder = {
      source  = "mckayla.dev/coder/coder"
      version = "1.0.0"
      # source  = "coder/coder"
      # version = "0.6.13"
    }
  }
}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_external_auth" "gitlab" {
  id       = "gitlab"
  optional = true
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}
