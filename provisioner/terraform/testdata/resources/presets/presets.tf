terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.3.0"
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
    expiration_policy {
      ttl = 86400
    }
    scheduling {
      timezone = "America/Los_Angeles"
      schedule {
        cron      = "* 8-18 * * 1-5"
        instances = 3
      }
      schedule {
        cron      = "* 8-14 * * 6"
        instances = 1
      }
    }
  }
}

resource "coder_agent" "dev" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}

