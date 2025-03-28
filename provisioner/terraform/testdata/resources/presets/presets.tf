terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.3.0-pre2"
    }
  }
}

module "this_is_external_module" {
  source = "./external-module"
}

data "coder_parameter" "sample" {
  name        = "Sample"
  type        = "string"
  description = "blah blah"
  default     = "ok"
}

data "coder_workspace_preset" "MyFirstProject" {
  name = "My First Project"
  parameters = {
    (data.coder_parameter.sample.name) = "A1B2C3"
  }
  prebuilds {
    instances = 4
  }
}

resource "coder_agent" "dev" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}

