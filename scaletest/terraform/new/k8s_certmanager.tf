locals {
  cert_manager_namespace                    = "cert-manager"
  cert_manager_helm_repo                    = "https://charts.jetstack.io"
  cert_manager_helm_chart                   = "cert-manager"
  cert_manager_release_name                 = "cert-manager"
  cert_manager_chart_version                = "1.12.2"
  cloudflare_issuer_private_key_secret_name = "cloudflare-issuer-private-key"
}

resource "kubernetes_secret" "cloudflare-api-key" {
  provider = kubernetes.primary

  metadata {
    name      = "cloudflare-api-key-secret"
    namespace = local.cert_manager_namespace
  }
  data = {
    api-token = var.cloudflare_api_token
  }
}

resource "kubernetes_namespace" "cert-manager-namespace" {
  provider = kubernetes.primary

  metadata {
    name = local.cert_manager_namespace
  }
}

resource "helm_release" "cert-manager" {
  provider = helm.primary

  repository = local.cert_manager_helm_repo
  chart      = local.cert_manager_helm_chart
  name       = local.cert_manager_release_name
  namespace  = kubernetes_namespace.cert-manager-namespace.metadata.0.name
  values = [<<EOF
installCRDs: true
EOF
  ]
}

resource "kubectl_manifest" "cloudflare-cluster-issuer" {
  provider = kubectl.primary

  depends_on = [ helm_release.cert-manager ]
    yaml_body = <<YAML
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: cloudflare-issuer
spec:
  acme:
    email: ${var.cloudflare_email}
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: ${local.cloudflare_issuer_private_key_secret_name}
    solvers:
      - dns01:
          cloudflare:
            apiTokenSecretRef:
              name: ${kubernetes_secret.cloudflare-api-key.metadata.0.name}
              key: api-token
YAML
}
