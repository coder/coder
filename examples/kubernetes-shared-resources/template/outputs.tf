output "namespace" {
  value = kubernetes_namespace_v1.workspaces.metadata[0].name
}

output "template_id" {
  value = data.coder_template.current.id
}

output "foundation_id" {
  value = data.coder_foundation.current.id
}
