terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.0"
    }
  }
}

resource "coder_agent" "dev1" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "dev2" {
  os   = "darwin"
  arch = "amd64"
}

resource "coder_agent" "dev3" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1,
    coder_agent.dev2,
    coder_agent.dev3
  ]
}
