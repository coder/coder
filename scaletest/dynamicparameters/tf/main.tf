# Cache busting string so each copy of the template is unique: {{.RandomString}}
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.5.3"
    }
  }
}

locals {
  one_options = {
    "A" = ["AA", "AB"]
    # spellchecker:ignore-next-line
    "B" = ["BA", "BB"]
  }

  three_options = {
    "AA" = ["AAA", "AAB"]
    "AB" = ["ABA", "ABB"]
    # spellchecker:ignore-next-line
    "BA" = ["BAA", "BAB"]
    "BB" = ["BBA", "BBB"]
  }

  username = data.coder_workspace_owner.me.name
}

data "coder_workspace_owner" "me" {}

data "coder_parameter" "zero" {
  name         = "zero"
  display_name = "Root"
  description  = "Hello ${local.username}, pick your next parameter using this `dropdown` parameter."
  form_type    = "dropdown"
  mutable      = true
  default      = "A"

  option {
    value = "A"
    name  = "A"
  }

  option {
    value = "B"
    name  = "B"
  }
}

data "coder_parameter" "one" {

  name         = "One"
  display_name = "Level One"
  description  = "This is the first level."

  type      = "list(string)"
  form_type = "multi-select"
  order     = 2
  mutable   = true
  default   = "[\"${local.one_options[data.coder_parameter.zero.value][0]}\"]"

  dynamic "option" {
    for_each = local.one_options[data.coder_parameter.zero.value]
    content {
      name  = option.value
      value = option.value
    }
  }
}

module "two" {
  source = "./modules/two"

  one_value = data.coder_parameter.one.value
}

data "coder_parameter" "three" {

  name         = "Three"
  display_name = "Level Three"
  description  = "This is the third level."

  type      = "string"
  form_type = "radio"
  order     = 4
  mutable   = true
  default   = local.three_options[module.two.two_value][0]

  dynamic "option" {
    for_each = local.three_options[module.two.two_value]
    content {
      name  = option.value
      value = option.value
    }
  }
}

data "coder_parameter" "four" {
  name         = "four"
  display_name = "Level Four"
  description  = "This is the last level."
  order        = 5

  type      = "string"
  form_type = "radio"
  default   = "a_fake_value_to_satisfy_import"

  option {
    name  = format("%s-%s", local.username, data.coder_parameter.three.value)
    value = "a_fake_value_to_satisfy_import"
  }

  dynamic "option" {
    for_each = data.coder_workspace_owner.me.rbac_roles
    content {
      name  = format("%s-%s", option.value.name, data.coder_parameter.three.value)
      value = option.value.name
    }
  }
}
