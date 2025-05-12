terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "group" {
  name    = "group"
  default = try(data.coder_workspace_owner.me.groups[0], "")
  dynamic "option" {
    for_each = concat(data.coder_workspace_owner.me.groups, "bloob")
    content {
      name  = option.value
      value = option.value
    }
  }
}
