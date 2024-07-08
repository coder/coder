terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
  }
}

data "coder_parameter" "sample" {
  name        = "Sample"
  type        = "string"
  description = "blah blah"
  default     = "ok"
  order       = 99
}

data "coder_parameter" "example" {
  name  = "Example"
  type  = "string"
  order = 55
}

resource "coder_agent" "dev" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}
