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

variable "agent_name" {
  type    = string
  default = ""
}

variable "folder" {
  type = string
}

data "coder_workspace" "me" {}

locals {
  workspace_name = lower(data.coder_workspace.me.name)
  agent_name     = lower(var.agent_name)
  hostname       = var.agent_name != "" ? "${local.agent_name}.${local.workspace_name}.me.coder" : "${local.workspace_name}.coder"
}

resource "coder_app" "zed" {
  agent_id     = var.agent_id
  display_name = "Zed"
  slug         = "zed"
  icon         = "/icon/zed.svg"
  external     = true
  url          = "zed://ssh/${local.hostname}/${var.folder}"
}
