terraform {
  required_version = ">= 1.0"

  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 0.17"
    }
  }
}

locals {
  jetbrains_ides = {
    "GO" = {
      icon          = "/icon/goland.svg",
      name          = "GoLand",
      identifier    = "GO",
    },
    "WS" = {
      icon          = "/icon/webstorm.svg",
      name          = "WebStorm",
      identifier    = "WS",
    },
    "IU" = {
      icon          = "/icon/intellij.svg",
      name          = "IntelliJ IDEA Ultimate",
      identifier    = "IU",
    },
    "PY" = {
      icon          = "/icon/pycharm.svg",
      name          = "PyCharm Professional",
      identifier    = "PY",
    },
    "CL" = {
      icon          = "/icon/clion.svg",
      name          = "CLion",
      identifier    = "CL",
    },
    "PS" = {
      icon          = "/icon/phpstorm.svg",
      name          = "PhpStorm",
      identifier    = "PS",
    },
    "RM" = {
      icon          = "/icon/rubymine.svg",
      name          = "RubyMine",
      identifier    = "RM",
    },
    "RD" = {
      icon          = "/icon/rider.svg",
      name          = "Rider",
      identifier    = "RD",
    },
    "RR" = {
      icon          = "/icon/rustrover.svg",
      name          = "RustRover",
      identifier    = "RR"
    }
  }

  icon          = local.jetbrains_ides[data.coder_parameter.jetbrains_ide.value].icon
  display_name  = local.jetbrains_ides[data.coder_parameter.jetbrains_ide.value].name
  identifier    = data.coder_parameter.jetbrains_ide.value
}

data "coder_parameter" "jetbrains_ide" {
  type         = "string"
  name         = "jetbrains_ide"
  display_name = "JetBrains IDE"
  icon         = "/icon/gateway.svg"
  mutable      = true
  default      = sort(keys(local.jetbrains_ides))[0]

  dynamic "option" {
    for_each = local.jetbrains_ides
    content {
      icon  = option.value.icon
      name  = option.value.name
      value = option.key
    }
  }
}

output "identifier" {
  value = local.identifier
}

output "display_name" {
  value = local.display_name
}

output "icon" {
  value = local.icon
}
