terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
  }
}

data "coder_git_auth" "github" {
  id = "github"
}

data "coder_git_auth" "gitlab" {
  id = "gitlab"
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
