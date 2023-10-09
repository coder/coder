data "google_client_config" "default" {}

locals {
  coder_url                 = var.coder_access_url
  coder_admin_email         = "admin@coder.com"
  coder_admin_user          = "coder"
  coder_helm_repo           = "https://helm.coder.com/v2"
  coder_helm_chart          = "coder"
  coder_namespace           = "coder-${var.name}"
  coder_release_name        = var.name
  provisionerd_helm_chart   = "coder-provisioner"
  provisionerd_release_name = "${var.name}-provisionerd"
}

resource "kubernetes_namespace" "coder_namespace" {
  metadata {
    name = local.coder_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "random_password" "provisionerd_psk" {
  length = 26
}

resource "kubernetes_secret" "coder-db" {
  type = "Opaque"
  metadata {
    name      = "coder-db-url"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    url = var.coder_db_url
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "kubernetes_secret" "provisionerd_psk" {
  type = "Opaque"
  metadata {
    name      = "coder-provisioner-psk"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    psk = random_password.provisionerd_psk.result
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

# OIDC secret needs to be manually provisioned for now.
data "kubernetes_secret" "coder_oidc" {
  metadata {
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
    name      = "coder-oidc"
  }
}

resource "kubernetes_manifest" "coder_certificate" {
  manifest = {
    apiVersion = "cert-manager.io/v1"
    kind       = "Certificate"
    metadata = {
      name      = "${var.name}"
      namespace = kubernetes_namespace.coder_namespace.metadata.0.name
    }
    spec = {
      secretName = "${var.name}-tls"
      dnsNames   = regex("https?://([^/]+)", local.coder_url)
      issuerRef = {
        name = kubernetes_manifest.cloudflare-cluster-issuer.manifest.metadata.name
        kind = "ClusterIssuer"
      }
    }
  }
}

data "kubernetes_secret" "coder_tls" {
  metadata {
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
    name      = "${var.name}-tls"
  }
  depends_on = [kubernetes_manifest.coder_certificate]
}

resource "helm_release" "coder-chart" {
  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = kubernetes_namespace.coder_namespace.metadata.0.name
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_coder}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_ACCESS_URL"
      value: "${local.coder_url}"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PG_CONNECTION_URL"
      valueFrom:
        secretKeyRef:
          name: "${kubernetes_secret.coder-db.metadata.0.name}"
          key: url
    - name: "CODER_PPROF_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_AGENT_STATS"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_DB_METRICS"
      value: "true"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_EXPERIMENTS"
      value: "${var.coder_experiments}"
    - name: "CODER_DANGEROUS_DISABLE_RATE_LIMITS"
      value: "true"
    # Disabling built-in provisioner daemons
    - name: "CODER_PROVISIONER_DAEMONS"
      value: "0"
    - name: CODER_PROVISIONER_DAEMON_PSK
      valueFrom:
        secretKeyRef:
          key: psk
          name: "${kubernetes_secret.provisionerd_psk.metadata.0.name}"
    # Enable OIDC
    - name: "CODER_OIDC_ISSUER_URL"
      valueFrom:
        secretKeyRef:
          key: issuer-url
          name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    - name: "CODER_OIDC_EMAIL_DOMAIN"
      valueFrom:
        secretKeyRef:
          key: email-domain
          name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    - name: "CODER_OIDC_CLIENT_ID"
      valueFrom:
        secretKeyRef:
          key: client-id
          name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    - name: "CODER_OIDC_CLIENT_SECRET"
      valueFrom:
        secretKeyRef:
          key: client-secret
          name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    # Send OTEL traces to the cluster-local collector to sample 10%
    - name: "OTEL_EXPORTER_OTLP_ENDPOINT"
      value: "http://${kubernetes_manifest.otel-collector.manifest.metadata.name}-collector.${kubernetes_namespace.coder_namespace.metadata.0.name}.svc.cluster.local:4317"
    - name: "OTEL_TRACES_SAMPLER"
      value: parentbased_traceidratio
    - name: "OTEL_TRACES_SAMPLER_ARG"
      value: "0.1"
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${var.coder_replicas}"
  resources:
    requests:
      cpu: "${var.coder_cpu_request}"
      memory: "${var.coder_mem_request}"
    limits:
      cpu: "${var.coder_cpu_limit}"
      memory: "${var.coder_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  service:
    enable: true
    sessionAffinity: None
    loadBalancerIP: "${var.coder_address}"
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

resource "helm_release" "provisionerd-chart" {
  repository = local.coder_helm_repo
  chart      = local.provisionerd_helm_chart
  name       = local.provisionerd_release_name
  version    = var.provisionerd_chart_version
  namespace  = kubernetes_namespace.coder_namespace.metadata.0.name
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_coder}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_URL"
      value: "${local.coder_url}"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROVISIONERD_TAGS"
      value = "socpe=organization"
  image:
    repo: ${var.provisionerd_image_repo}
    tag: ${var.provisionerd_image_tag}
  replicaCount: "${var.provisionerd_replicas}"
  resources:
    requests:
      cpu: "${var.provisionerd_cpu_request}"
      memory: "${var.provisionerd_mem_request}"
    limits:
      cpu: "${var.provisionerd_cpu_limit}"
      memory: "${var.provisionerd_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

resource "local_file" "kubernetes_template" {
  filename = "${path.module}/../.coderv2/templates/kubernetes/main.tf"
  content  = <<EOF
    terraform {
      required_providers {
        coder = {
          source  = "coder/coder"
          version = "~> 0.7.0"
        }
        kubernetes = {
          source  = "hashicorp/kubernetes"
          version = "~> 2.18"
        }
      }
    }

    provider "coder" {}

    provider "kubernetes" {
      config_path = null # always use host
    }

    data "coder_workspace" "me" {}

    resource "coder_agent" "main" {
      os                     = "linux"
      arch                   = "amd64"
      startup_script_timeout = 180
      startup_script         = ""
    }

    resource "kubernetes_pod" "main" {
      count = data.coder_workspace.me.start_count
      metadata {
        name      = "coder-$${lower(data.coder_workspace.me.owner)}-$${lower(data.coder_workspace.me.name)}"
        namespace = "${local.coder_namespace}"
        labels = {
          "app.kubernetes.io/name"     = "coder-workspace"
          "app.kubernetes.io/instance" = "coder-workspace-$${lower(data.coder_workspace.me.owner)}-$${lower(data.coder_workspace.me.name)}"
        }
      }
      spec {
        security_context {
          run_as_user = "1000"
          fs_group    = "1000"
        }
        container {
          name              = "dev"
          image             = "${var.workspace_image}"
          image_pull_policy = "Always"
          command           = ["sh", "-c", coder_agent.main.init_script]
          security_context {
            run_as_user = "1000"
          }
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          resources {
            requests = {
              "cpu"    = "${var.workspace_cpu_request}"
              "memory" = "${var.workspace_mem_request}"
            }
            limits = {
              "cpu"    = "${var.workspace_cpu_limit}"
              "memory" = "${var.workspace_mem_limit}"
            }
          }
        }

        affinity {
          node_affinity {
            required_during_scheduling_ignored_during_execution {
              node_selector_term {
                match_expressions {
                  key = "cloud.google.com/gke-nodepool"
                  operator = "In"
                  values = ["${var.kubernetes_nodepool_workspaces}"]
                }
              }
            }
          }
        }
      }
    }
  EOF
}

resource "local_file" "output_vars" {
  filename = "${path.module}/../../.coderv2/url"
  content  = local.coder_url
}

output "coder_url" {
  description = "URL of the Coder deployment"
  value       = local.coder_url
}
