terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.6"
    }
  }
}

data "coder_parameter" "example" {
  name = "Example"
  type = "string"
  option {
    name  = "First Option"
    value = "first"
  }
  option {
    name  = "Second Option"
    value = "second"
  }
}

resource "coder_agent" "dev" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}
