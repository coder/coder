terraform {
  required_providers {
    coderd = {
      source = "coder/coderd"
    }
  }
  backend "gcs" {
    bucket = "coder-dogfood-tf-state"
    prefix = "/templates/coder-envbuilder"
  }
}

data "coderd_organization" "default" {
  is_default = true
}

data "coderd_user" "machine" {
  username = "machine"
}

variable "CODER_TEMPLATE_VERSION" {
  type = string
}

variable "CODER_TEMPLATE_DIR" {
  type = string
}

variable "CODER_TEMPLATE_MESSAGE" {
  type = string
}

resource "coderd_template" "coder-envbuilder" {
  name            = "coder-envbuilder"
  display_name    = "Write Coder on Coder using Envbuilder"
  description     = "Write Coder on Coder using a workspace built by Envbuilder."
  icon            = "/emojis/1f3d7.png" # 🏗️
  organization_id = "703f72a1-76f6-4f89-9de6-8a3989693fe5"
  versions = [
    {
      name      = var.CODER_TEMPLATE_VERSION
      message   = var.CODER_TEMPLATE_MESSAGE
      directory = var.CODER_TEMPLATE_DIR
      active    = true
      tf_vars = [
        {
          # clusters/dogfood-v2/coder/provisioner/configs/values.yaml#L191-L194
          name  = "envbuilder_cache_dockerconfigjson_path"
          value = "/home/coder/envbuilder-cache-dockerconfig.json"
        }
      ]
    }
  ]
  acl = {
    groups = [{
      id   = data.coderd_organization.default.id
      role = "use"
    }]
    users = [{
      id   = data.coderd_user.machine.id
      role = "admin"
    }]
  }
  activity_bump_ms                  = 10800000
  allow_user_auto_start             = true
  allow_user_auto_stop              = true
  allow_user_cancel_workspace_jobs  = false
  auto_start_permitted_days_of_week = ["friday", "monday", "saturday", "sunday", "thursday", "tuesday", "wednesday"]
  auto_stop_requirement = {
    days_of_week = ["sunday"]
    weeks        = 1
  }
  default_ttl_ms                 = 28800000
  deprecation_message            = null
  failure_ttl_ms                 = 604800000
  require_active_version         = true
  time_til_dormant_autodelete_ms = 7776000000
  time_til_dormant_ms            = 8640000000
}
