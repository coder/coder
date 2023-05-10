data "google_client_config" "default" {}

locals {
  coder_helm_repo    = "https://helm.coder.com/v2"
  coder_helm_chart   = "coder"
  coder_release_name = "coder-${var.name}"
  coder_namespace    = "coder-${var.name}"
}

provider "kubernetes" {
  host                   = "https://${google_container_cluster.primary.endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.primary.master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

provider "helm" {
  kubernetes {
    host                   = "https://${google_container_cluster.primary.endpoint}"
    cluster_ca_certificate = base64decode(google_container_cluster.primary.master_auth.0.cluster_ca_certificate)
    token                  = data.google_client_config.default.access_token
  }
}

resource "kubernetes_namespace" "coder_namespace" {
  metadata {
    name = local.coder_namespace
  }
  depends_on = [
    google_container_node_pool.coder
  ]
}

resource "random_password" "postgres-admin-password" {
  length = 12
}

resource "random_password" "coder-postgres-password" {
  length = 12
}

resource "kubernetes_secret" "coder-db" {
  type = "" # Opaque
  metadata {
    name      = "coder-db-url"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    url = "postgres://coder:${urlencode(random_password.coder-postgres-password.result)}@${google_sql_database_instance.db.private_ip_address}/${google_sql_database.coder.name}?sslmode=disable"
  }
}

resource "tls_private_key" "coder" {
  algorithm = "ED25519"
}

resource "tls_self_signed_cert" "coder" {
  private_key_pem = tls_private_key.coder.private_key_pem

  subject {
    common_name = "${local.coder_release_name}.${local.coder_namespace}.svc.cluster.local"
  }

  allowed_uses = ["server_auth", "digital_signature", "data_encipherment", "key_agreement", "key_encipherment"]

  # 1 year
  validity_period_hours = 8760

  dns_names = [
    "${local.coder_release_name}.${local.coder_namespace}.svc.cluster.local",
    "${local.coder_release_name}.${local.coder_namespace}",
    "${local.coder_release_name}",
  ]
}

resource "kubernetes_secret" "coder-tls" {
  type = "kubernetes.io/tls"
  metadata {
    name      = "coder-tls"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }

  data = {
    "tls.crt" = tls_self_signed_cert.coder.cert_pem
    "tls.key" = tls_private_key.coder.private_key_pem
  }
}

resource "kubernetes_secret" "coder-ca" {
  type = "Opaque"
  metadata {
    name      = "coder-ca"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    "ca.crt" = "${tls_self_signed_cert.coder.cert_pem}"
  }
}

resource "helm_release" "coder-chart" {
  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = kubernetes_namespace.coder_namespace.metadata.0.name
  depends_on = [
    google_container_node_pool.coder,
  ]
  values = [<<EOF
coder:
  env:
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PG_CONNECTION_URL"
      valueFrom:
        secretKeyRef:
          name: "${kubernetes_secret.coder-db.metadata.0.name}"
          key: url
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_VERBOSE"
      value: "true"
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${var.coder_replicas}"
  resources:
    requests:
      cpu: "${var.coder_cpu}"
      memory: "${var.coder_mem}"
    limits:
      cpu: "${var.coder_cpu}"
      memory: "${var.coder_mem}"
  securityContext:
    readOnlyRootFilesystem: true
  service:
    enable: true
  tls:
    secretNames:
    - "${kubernetes_secret.coder-tls.metadata.0.name}"
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
  extraTemplates:
  - |
    apiVersion: monitoring.googleapis.com/v1
    kind: PodMonitoring
    metadata:
      namespace: ${kubernetes_namespace.coder_namespace.metadata.0.name}
      name: coder-monitoring
    spec:
      selector:
        matchLabels:
          app.kubernetes.io/name: coder
      endpoints:
      - port: prometheus-http
        interval: 30s

EOF
  ]
}
