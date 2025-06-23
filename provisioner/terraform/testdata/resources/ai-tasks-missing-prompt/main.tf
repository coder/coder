terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.0.0"
    }
  }
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = data.coder_provisioner.me.os
}

resource "coder_ai_task" "a" {
  sidebar_app {
    id = "5ece4674-dd35-4f16-88c8-82e40e72e2fd" # fake ID to satisfy requirement, irrelevant otherwise
  }
}
