terraform {
  required_version = ">= 1.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 0.17"
    }
  }
}

variable "agent_id" {
  type = string
}

variable "folder" {
  type = string
}

data "coder_workspace" "me" {}

resource "coder_app" "zed" {
  agent_id     = var.agent_id
  display_name = "Zed Editor"
  slug         = "zed"
  icon         = "/icon/zed.svg"
  external     = true
  url          = "zed://ssh/coder.${lower(data.coder_workspace.me.name)}/${var.folder}"
}
