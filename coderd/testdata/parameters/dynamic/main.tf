terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      version = "2.5.3"
    }
  }
}

data "coder_workspace_owner" "me" {}

locals {
  isAdmin = contains(data.coder_workspace_owner.me.groups, "admin")
}

data "coder_parameter" "isAdmin" {
  name         = "isAdmin"
  type         = "bool"
  form_type    = "switch"
  default      = local.isAdmin
  order        = 1
}
