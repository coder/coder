# Template-scoped root: managed once, independently of workspace builds.
# This example needs no cluster or cloud credentials.
terraform {
  required_version = ">= 1.9.0, < 2.0.0"
}

resource "terraform_data" "settings" {
  input = {
    namespace = "workspaces"
    greeting  = "Hello from template resources"
  }
}

output "namespace" {
  value = terraform_data.settings.output.namespace
}

output "greeting" {
  value = terraform_data.settings.output.greeting
}

output "structured" {
  value = {
    enabled = true
    ports   = [443, 3100]
  }
}

# Sensitive outputs remain in template state, not workspace snapshots.
output "private_setting" {
  value     = "demo-only-not-a-real-secret"
  sensitive = true
}
