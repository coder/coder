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

data "coder_parameter" "child_first_parameter_from_module" {
  name        = "First parameter from child module"
  mutable     = true
  type        = "string"
  description = "First parameter from child module"
  default     = "abcdef"
}

data "coder_parameter" "child_second_parameter_from_module" {
  name        = "Second parameter from child module"
  mutable     = true
  type        = "string"
  description = "Second parameter from child module"
  default     = "ghijkl"
}
