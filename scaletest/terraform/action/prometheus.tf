locals {
  prometheus_helm_repo             = "https://charts.bitnami.com/bitnami"
  prometheus_helm_chart            = "kube-prometheus"
  prometheus_exporter_helm_repo    = "https://prometheus-community.github.io/helm-charts"
  prometheus_exporter_helm_chart   = "prometheus-postgres-exporter"
  prometheus_release_name          = "prometheus"
  prometheus_exporter_release_name = "prometheus-postgres-exporter"
  prometheus_namespace             = "prometheus"
  prometheus_remote_write_send_interval = "15s"
  prometheus_remote_write_metrics_regex = ".*"
}

resource "kubernetes_namespace" "prometheus_namespace_primary" {
  provider = kubernetes.primary

  metadata {
    name = local.prometheus_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "helm_release" "prometheus_chart_primary" {
  provider = helm.primary

  repository = local.prometheus_helm_repo
  chart      = local.prometheus_helm_chart
  name       = local.prometheus_release_name
  namespace  = kubernetes_namespace.prometheus_namespace_primary.metadata.0.name
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
            values: ["${google_container_node_pool.node_pool["primary_misc"].name}"]
operator:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["primary_misc"].name}"]
prometheus:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["primary_misc"].name}"]
  externalLabels:
    cluster: "primary"
  persistence:
    enabled: true
    storageClass: standard
  remoteWrite:
    - url: "${var.prometheus_remote_write_url}"
      tlsConfig:
        insecureSkipVerify: true
      writeRelabelConfigs:
        - sourceLabels: [__name__]
          regex: "${local.prometheus_remote_write_metrics_regex}"
          action: keep
      metadataConfig:
        sendInterval: "${local.prometheus_remote_write_send_interval}"
  EOF
  ]
}
