terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

data "coder_parameter" "number_example_min_max" {
  name    = "number_example_min_max"
  type    = "number"
  default = 4
  validation {
    min = 3
    max = 6
  }
}

data "coder_parameter" "number_example_min" {
  name    = "number_example_min"
  type    = "number"
  default = 4
  validation {
    min = 3
  }
}

data "coder_parameter" "number_example_min_zero" {
  name    = "number_example_min_zero"
  type    = "number"
  default = 4
  validation {
    min = 0
  }
}

data "coder_parameter" "number_example_max" {
  name    = "number_example_max"
  type    = "number"
  default = 4
  validation {
    max = 6
  }
}

data "coder_parameter" "number_example_max_zero" {
  name    = "number_example_max_zero"
  type    = "number"
  default = -3
  validation {
    max = 0
  }
}

data "coder_parameter" "number_example" {
  name      = "number_example"
  type      = "number"
  default   = 4
  mutable   = true
  ephemeral = true
}

resource "coder_agent" "dev" {
  os   = "windows"
  arch = "arm64"
}

resource "null_resource" "dev" {
  depends_on = [coder_agent.dev]
}
