terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "string" {
  name = "string"
  type = "string"
  validation {
    error = "All messages must start with 'Hello'"
    regex = "^Hello"
  }
}
