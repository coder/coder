// Base case for workspace tags + parameters.
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

variable "one" {
  default = "alice"
  type    = string
}


data "coder_parameter" "variable_values" {
  name        = "variable_values"
  description = "Just to show the variable values"
  type        = "string"
  default     = var.one

  option {
    name  = "one"
    value = var.one
  }
}
