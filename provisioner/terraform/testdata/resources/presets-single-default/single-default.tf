terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.3.0"
    }
  }
}

data "coder_parameter" "instance_type" {
  name        = "instance_type"
  type        = "string"
  description = "Instance type"
  default     = "t3.micro"
}

data "coder_workspace_preset" "development" {
  name    = "development"
  default = true
  parameters = {
    (data.coder_parameter.instance_type.name) = "t3.micro"
  }
  prebuilds {
    instances = 1
  }
}

data "coder_workspace_preset" "production" {
  name    = "production"
  default = false
  parameters = {
    (data.coder_parameter.instance_type.name) = "t3.large"
  }
  prebuilds {
    instances = 2
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}
