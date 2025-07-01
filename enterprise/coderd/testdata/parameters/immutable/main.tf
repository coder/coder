terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "immutable" {
  name    = "immutable"
  type    = "string"
  mutable = false
  default = "Hello World"
}
