terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "required" {
  name      = "required"
  type      = "string"
  mutable   = true
  ephemeral = true
}


data "coder_parameter" "defaulted" {
  name      = "defaulted"
  type      = "string"
  mutable   = true
  ephemeral = true
  default   = "original"
}
