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

data "coder_template_outputs" "current" {}

locals {
  shared = data.coder_template_outputs.current.outputs
}

resource "terraform_data" "workspace_settings" {
  input = {
    namespace        = local.shared.namespace
    greeting         = local.shared.greeting
    template_version = data.coder_template_outputs.current.revision_id
    structured       = local.shared.structured
  }
}

output "workspace_settings" {
  value = terraform_data.workspace_settings.output
}
