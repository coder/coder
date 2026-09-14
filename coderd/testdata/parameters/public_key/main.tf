terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "public_key" {
  name    = "public_key"
  default = data.coder_workspace_owner.me.ssh_public_key
}
