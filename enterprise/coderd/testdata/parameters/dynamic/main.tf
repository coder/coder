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
  order = 1
}

data "coder_parameter" "adminonly" {
  count = local.isAdmin ? 1 : 0
  name         = "adminonly"
  form_type = "input"
  type      = "string"
  default   = "I am an admin!"
  order = 2
}


data "coder_parameter" "groups" {
  name         = "groups"
  type         = "list(string)"
  form_type    = "multi-select"
  default = jsonencode([data.coder_workspace_owner.me.groups[0]])
  order = 50

  dynamic "option" {
    for_each = data.coder_workspace_owner.me.groups
    content {
      name  = option.value
      value = option.value
    }
  }
}

locals {
  colors = {
    "red": ["apple", "ruby"]
    "yellow": ["banana"]
    "blue": ["ocean", "sky"]
  }
}

data "coder_parameter" "colors" {
  name         = "colors"
  type         = "list(string)"
  form_type    = "multi-select"
  order = 100

  dynamic "option" {
    for_each = keys(local.colors)
    content {
      name  = option.value
      value = option.value
    }
  }
}

locals {
  selected = jsondecode(data.coder_parameter.colors.value)
  things = flatten([
    for color in local.selected : local.colors[color]
  ])
}

data "coder_parameter" "thing" {
  name         = "thing"
  type         = "string"
  form_type    = "dropdown"
  order = 101

  dynamic "option" {
    for_each = local.things
    content {
      name  = option.value
      value = option.value
    }
  }
}
