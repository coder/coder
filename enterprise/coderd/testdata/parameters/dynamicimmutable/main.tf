terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "isimmutable" {
  name    = "isimmutable"
  type    = "bool"
  mutable = true
  default = "true"
}

data "coder_parameter" "immutable" {
  name    = "immutable"
  type    = "string"
  mutable = data.coder_parameter.isimmutable.value == "false"
  default = "Hello World"
}
