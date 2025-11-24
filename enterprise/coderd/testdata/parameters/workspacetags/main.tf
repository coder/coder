terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}


variable "stringvar" {
  type    = string
  default = "bar"
}

variable "numvar" {
  type    = number
  default = 42
}

variable "boolvar" {
  type    = bool
  default = true
}

data "coder_parameter" "stringparam" {
  name    = "stringparam"
  type    = "string"
  default = "foo"
}

data "coder_parameter" "stringparamref" {
  name    = "stringparamref"
  type    = "string"
  default = data.coder_parameter.stringparam.value
}

data "coder_parameter" "numparam" {
  name    = "numparam"
  type    = "number"
  default = 7
}

data "coder_parameter" "boolparam" {
  name    = "boolparam"
  type    = "bool"
  default = true
}

data "coder_parameter" "listparam" {
  name    = "listparam"
  type    = "list(string)"
  default = jsonencode(["a", "b"])
}

data "coder_workspace_tags" "tags" {
  tags = {
    "function"    = format("param is %s", data.coder_parameter.stringparamref.value)
    "stringvar"   = var.stringvar
    "numvar"      = var.numvar
    "boolvar"     = var.boolvar
    "stringparam" = data.coder_parameter.stringparam.value
    "numparam"    = data.coder_parameter.numparam.value
    "boolparam"   = data.coder_parameter.boolparam.value
    "listparam"   = data.coder_parameter.listparam.value
  }
}
