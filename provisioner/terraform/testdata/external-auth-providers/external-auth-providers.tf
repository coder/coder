terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.16.0"
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
