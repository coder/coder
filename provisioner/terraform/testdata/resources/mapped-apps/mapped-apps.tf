terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

locals {
  apps_map = {
    "app1" = {
      name = "app1"
    }
    "app2" = {
      name = "app2"
    }
  }
}

resource "coder_app" "apps" {
  for_each = local.apps_map

  agent_id     = coder_agent.dev.id
  slug         = each.key
  display_name = each.value.name
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev
  ]
}
