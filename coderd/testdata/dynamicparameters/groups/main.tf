terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data coder_workspace_owner "me" {}

output "groups" {
  value = data.coder_workspace_owner.me.groups
}

data "coder_parameter" "group" {
  name = "group"
  default = try(data.coder_workspace_owner.me.groups[0], "")
  dynamic "option" {
    for_each = data.coder_workspace_owner.me.groups
    content {
      name  = option.value
      value = option.value
    }
  }
}
