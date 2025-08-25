locals {
  coder_certs_namespace = "coder-certs"
}

# These certificates are managed by flux and cert-manager.
data "kubernetes_secret" "coder_tls" {
  for_each = local.deployments
  provider = kubernetes.observability
  metadata {
    name      = "coder-${var.name}-${each.key}-tls"
    namespace = local.coder_certs_namespace
  }
}
