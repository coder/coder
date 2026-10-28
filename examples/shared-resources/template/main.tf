# Shared root: managed by a Foundation, independently of workspace builds.
# This example needs no cluster or cloud credentials.
terraform {
  required_version = ">= 1.9.0, < 2.0.0"
}

resource "terraform_data" "settings" {
  input = {
    namespace = "workspaces"
    greeting  = "Hello from shared resources"
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

# Public identity keys remain usable even when the snapshot also contains secrets.
output "public_set" {
  value = toset(["http", "https"])
}

# Sensitive root outputs are available through coder_foundation.sensitive_outputs.
output "private_setting" {
  value     = "demo-only-not-a-real-secret"
  sensitive = true
}
