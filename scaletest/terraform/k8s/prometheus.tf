locals {
  prometheus_helm_repo             = "https://charts.bitnami.com/bitnami"
  prometheus_helm_chart            = "kube-prometheus"
  prometheus_exporter_helm_repo    = "https://prometheus-community.github.io/helm-charts"
  prometheus_exporter_helm_chart   = "prometheus-postgres-exporter"
  prometheus_release_name          = "prometheus"
  prometheus_exporter_release_name = "prometheus-postgres-exporter"
  prometheus_namespace             = "prometheus"
  prometheus_remote_write_enabled  = var.prometheus_remote_write_password != ""
}

# Create a namespace to hold our Prometheus deployment.
resource "kubernetes_namespace" "prometheus_namespace" {
  metadata {
    name = local.prometheus_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

# Create a secret to store the remote write key
resource "kubernetes_secret" "prometheus-credentials" {
  count = local.prometheus_remote_write_enabled ? 1 : 0
  type  = "kubernetes.io/basic-auth"
  metadata {
    name      = "prometheus-credentials"
    namespace = kubernetes_namespace.prometheus_namespace.metadata.0.name
  }

  data = {
    username = var.prometheus_remote_write_user
    password = var.prometheus_remote_write_password
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

# Install Prometheus using the Bitnami Prometheus helm chart.
resource "helm_release" "prometheus-chart" {
  repository = local.prometheus_helm_repo
  chart      = local.prometheus_helm_chart
  name       = local.prometheus_release_name
  namespace  = kubernetes_namespace.prometheus_namespace.metadata.0.name
  values = [<<EOF
alertmanager:
  enabled: false
blackboxExporter:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_misc}"]
operator:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_misc}"]
prometheus:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_misc}"]
  externalLabels:
    cluster: "${var.prometheus_external_label_cluster}"
  persistence:
    enabled: true
    storageClass: standard
%{if local.prometheus_remote_write_enabled~}
  remoteWrite:
    - url: "${var.prometheus_remote_write_url}"
      basicAuth:
        username:
          name: "${kubernetes_secret.prometheus-credentials[0].metadata[0].name}"
          key: username
        password:
          name: "${kubernetes_secret.prometheus-credentials[0].metadata[0].name}"
          key: password
      tlsConfig:
        insecureSkipVerify: ${var.prometheus_remote_write_insecure_skip_verify}
      writeRelabelConfigs:
        - sourceLabels: [__name__]
          regex: "${var.prometheus_remote_write_metrics_regex}"
          action: keep
      metadataConfig:
        sendInterval: "${var.prometheus_remote_write_send_interval}"
%{endif~}
  EOF
  ]
}

resource "kubernetes_secret" "prometheus-postgres-password" {
  type = "kubernetes.io/basic-auth"
  metadata {
    name      = "prometheus-postgres"
    namespace = kubernetes_namespace.prometheus_namespace.metadata.0.name
  }
  data = {
    username = var.prometheus_postgres_user
    password = var.prometheus_postgres_password
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

# Install Prometheus Postgres exporter helm chart
resource "helm_release" "prometheus-exporter-chart" {
  depends_on = [helm_release.prometheus-chart]
  repository = local.prometheus_exporter_helm_repo
  chart      = local.prometheus_exporter_helm_chart
  name       = local.prometheus_exporter_release_name
  namespace  = local.prometheus_namespace
  values = [<<EOF
affinity:
  nodeAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    nodeSelectorTerms:
    - matchExpressions:
      - key: "cloud.google.com/gke-nodepool"
        operator: "In"
        values: ["${var.kubernetes_nodepool_misc}"]
config:
  datasource:
    host: "${var.prometheus_postgres_host}"
    user: "${var.prometheus_postgres_user}"
    database: "${var.prometheus_postgres_dbname}"
    passwordSecret:
      name: "${kubernetes_secret.prometheus-postgres-password.metadata.0.name}"
      key: password
    autoDiscoverDatabases: true
serviceMonitor:
  enabled: true
  EOF
  ]
}

resource "kubernetes_manifest" "coder_monitoring" {
  depends_on = [helm_release.prometheus-chart]
  manifest = {
    apiVersion = "monitoring.coreos.com/v1"
    kind       = "PodMonitor"
    metadata = {
      namespace = kubernetes_namespace.coder_namespace.metadata.0.name
      name      = "coder-monitoring"
    }
    spec = {
      selector = {
        matchLabels = {
          "app.kubernetes.io/name" : "coder"
        }
      }
      podMetricsEndpoints = [
        {
          port     = "prometheus-http"
          interval = "30s"
        }
      ]
    }
  }
}
