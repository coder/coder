# Per-workspace root: each workspace has independent Terraform state.
terraform {
  required_version = ">= 1.9.0, < 2.0.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "= 2.19.0"
    }
  }
}

# The POC requires the companion provider's explicit development override.
provider "coder" {}

data "coder_foundation" "current" {}

locals {
  shared           = data.coder_foundation.current.outputs
  shared_sensitive = data.coder_foundation.current.sensitive_outputs
}

resource "terraform_data" "workspace_settings" {
  input = {
    namespace            = local.shared.namespace
    greeting             = local.shared.greeting
    foundation_snapshot  = data.coder_foundation.current.snapshot_id
    snapshot_foundation  = data.coder_foundation.current.id
    foundation_id        = data.coder_foundation.current.id
    foundation_name      = data.coder_foundation.current.name
    foundation_templates = data.coder_foundation.current.template_ids
    structured           = local.shared.structured
    private_setting      = local.shared_sensitive.private_setting
  }
}

output "workspace_settings" {
  value     = terraform_data.workspace_settings.output
  sensitive = true
}

# Public roots remain non-sensitive when another root in the snapshot is sensitive.
resource "terraform_data" "public_setting" {
  for_each = local.shared.public_set
  input    = each.value
}

output "namespace" {
  value = local.shared.namespace
}
