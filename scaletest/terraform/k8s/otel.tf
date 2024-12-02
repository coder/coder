# Terraform configuration for OpenTelemetry Operator

locals {
  otel_namespace              = "opentelemetry-operator-system"
  otel_operator_helm_repo     = "https://open-telemetry.github.io/opentelemetry-helm-charts"
  otel_operator_helm_chart    = "opentelemtry-operator"
  otel_operator_release_name  = "opentelemetry-operator"
  otel_operator_chart_version = "0.34.1"
}

resource "kubernetes_namespace" "otel-namespace" {
  metadata {
    name = local.otel_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "helm_release" "otel-operator" {
  repository = local.otel_operator_helm_repo
  chart      = local.otel_operator_helm_chart
  name       = local.otel_operator_release_name
  namespace  = kubernetes_namespace.otel-namespace.metadata.0.name
  # Default values
  values = []
}

resource "kubernetes_manifest" "otel-collector" {
  manifest = {
    apiVersion = "opentelemetry.io/v1alpha1"
    kind       = "OpenTelemetryCollector"
    metadata = {
      namespace = kubernetes_namespace.coder_namespace.metadata.0.name
      name      = "otel"
    }
    spec = {
      config = jsonencode({
        receivers = {
          otlp = {
            protocols : {
              grpc : {}
              http : {}
            }
          }
        }
        exporters = {
          googlecloud = {
            logging = {
              loglevel = "debug"
            }
          }
        }
        service = {
          pipelines = {
            traces = {
              receivers  = ["otlp"]
              processors = []
              exporters  = ["logging", "googlecloud"]
            }
          }
        }
        image    = "otel/open-telemetry-collector-contrib:latest"
        mode     = "deployment"
        replicas = 1
      })
    }
  }
}
