terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

