locals {
  prometheus_helm_repo                  = "oci://registry-1.docker.io/bitnamicharts"
  prometheus_helm_chart                 = "kube-prometheus"
  prometheus_release_name               = "prometheus"
  prometheus_namespace                  = "prometheus"
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
  values = [templatefile("${path.module}/prometheus_helm_values.tftpl", {
    nodepool                              = google_container_node_pool.node_pool["primary_misc"].name,
    cluster                               = "primary",
    prometheus_remote_write_url           = var.prometheus_remote_write_url,
    prometheus_remote_write_metrics_regex = local.prometheus_remote_write_metrics_regex,
    prometheus_remote_write_send_interval = local.prometheus_remote_write_send_interval,
  })]
}

resource "kubectl_manifest" "pod_monitor_primary" {
  provider = kubectl.primary

  yaml_body = <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: ${kubernetes_namespace.coder_primary.metadata.0.name}
  name: coder-monitoring
spec:
  selector:
    matchLabels:
      "app.kubernetes.io/name": coder
  podMetricsEndpoints:
    - port: prometheus-http
      interval: 30s
YAML

  depends_on = [helm_release.prometheus_chart_primary]
}

resource "kubernetes_namespace" "prometheus_namespace_europe" {
  provider = kubernetes.europe

  metadata {
    name = local.prometheus_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "helm_release" "prometheus_chart_europe" {
  provider = helm.europe

  repository = local.prometheus_helm_repo
  chart      = local.prometheus_helm_chart
  name       = local.prometheus_release_name
  namespace  = kubernetes_namespace.prometheus_namespace_europe.metadata.0.name
  values = [templatefile("${path.module}/prometheus_helm_values.tftpl", {
    nodepool                              = google_container_node_pool.node_pool["europe_misc"].name,
    cluster                               = "europe",
    prometheus_remote_write_url           = var.prometheus_remote_write_url,
    prometheus_remote_write_metrics_regex = local.prometheus_remote_write_metrics_regex,
    prometheus_remote_write_send_interval = local.prometheus_remote_write_send_interval,
  })]
}

resource "kubectl_manifest" "pod_monitor_europe" {
  provider = kubectl.europe

  yaml_body = <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: ${kubernetes_namespace.coder_europe.metadata.0.name}
  name: coder-monitoring
spec:
  selector:
    matchLabels:
      "app.kubernetes.io/name": coder
  podMetricsEndpoints:
    - port: prometheus-http
      interval: 30s
YAML

  depends_on = [helm_release.prometheus_chart_europe]
}

resource "kubernetes_namespace" "prometheus_namespace_asia" {
  provider = kubernetes.asia

  metadata {
    name = local.prometheus_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "helm_release" "prometheus_chart_asia" {
  provider = helm.asia

  repository = local.prometheus_helm_repo
  chart      = local.prometheus_helm_chart
  name       = local.prometheus_release_name
  namespace  = kubernetes_namespace.prometheus_namespace_asia.metadata.0.name
  values = [templatefile("${path.module}/prometheus_helm_values.tftpl", {
    nodepool                              = google_container_node_pool.node_pool["asia_misc"].name,
    cluster                               = "asia",
    prometheus_remote_write_url           = var.prometheus_remote_write_url,
    prometheus_remote_write_metrics_regex = local.prometheus_remote_write_metrics_regex,
    prometheus_remote_write_send_interval = local.prometheus_remote_write_send_interval,
  })]
}

resource "kubectl_manifest" "pod_monitor_asia" {
  provider = kubectl.asia

  yaml_body = <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: ${kubernetes_namespace.coder_asia.metadata.0.name}
  name: coder-monitoring
spec:
  selector:
    matchLabels:
      "app.kubernetes.io/name": coder
  podMetricsEndpoints:
    - port: prometheus-http
      interval: 30s
YAML

  depends_on = [helm_release.prometheus_chart_asia]
}
