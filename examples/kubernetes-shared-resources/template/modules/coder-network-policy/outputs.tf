output "policy_name" {
  description = "Managed namespaced policy name, or null when no templates are attached."
  value       = length(var.template_ids) == 0 ? null : local.policy_name
}

output "manifest" {
  description = "Terraform-owned desired policy for inspection and offline tests, or null when detached."
  value       = length(var.template_ids) == 0 ? null : local.manifest
}
