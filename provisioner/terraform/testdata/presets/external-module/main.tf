terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.22"
    }
  }
}

module "this_is_external_child_module" {
  source = "./child-external-module"
}

data "coder_parameter" "first_parameter_from_module" {
  name        = "First parameter from module"
  mutable     = true
  type        = "string"
  description = "First parameter from module"
  default     = "abcdef"
}

data "coder_parameter" "second_parameter_from_module" {
  name        = "Second parameter from module"
  mutable     = true
  type        = "string"
  description = "Second parameter from module"
  default     = "ghijkl"
}
