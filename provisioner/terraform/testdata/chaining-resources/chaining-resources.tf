terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "null_resource" "b" {
  depends_on = [
    coder_agent.main
  ]
}

resource "null_resource" "a" {
  depends_on = [
    null_resource.b
  ]
}
