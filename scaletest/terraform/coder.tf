data "google_client_config" "default" {}

locals {
  coder_helm_repo            = "https://helm.coder.com/v2"
  coder_helm_chart           = "coder"
  coder_release_name         = "coder-${var.name}"
  coder_namespace            = "coder-${var.name}"
  coder_admin_email          = "admin@coder.com"
  coder_admin_user           = "coder"
  coder_address              = "${google_compute_address.coder.address}"
  coder_url                  =  "https://${google_compute_address.coder.address}"
  rebuilt_workspace_image    =  "gcr.io/coder-dev-1/v2-loadtest/${var.name}/workspace:latest"
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
    url = "postgres://${google_sql_user.coder.name}:${urlencode(random_password.coder-postgres-password.result)}@${google_sql_database_instance.db.private_ip_address}/${google_sql_database.coder.name}?sslmode=disable"
  }
}

resource "tls_private_key" "coder" {
  algorithm = "RSA"
}

resource "tls_self_signed_cert" "coder" {
  private_key_pem = tls_private_key.coder.private_key_pem
  is_ca_certificate = true

  subject {
    common_name = "${local.coder_release_name}.${local.coder_namespace}.svc.cluster.local"
  }

  allowed_uses = ["digital_signature", "cert_signing", "crl_signing"]

  # 1 year
  validity_period_hours = 8760

  dns_names = [
    "${local.coder_release_name}.${local.coder_namespace}.svc.cluster.local",
    "${local.coder_release_name}.${local.coder_namespace}",
    "${local.coder_release_name}",
  ]

  ip_addresses = [
    google_compute_address.coder.address
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
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.workspaces.name}"]
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
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_ENABLE_TELEMETRY"
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
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_TLS_REDIRECT_HTTP_TO_HTTPS"
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
    loadBalancerIP: "${local.coder_address}"
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

resource "local_file" "url" {
  filename = "${path.module}/coder_url"
  content = "${local.coder_url}"
}

# Because we use a self-signed certificate, we need to also rebuild the base image.
resource "local_file" "workspace_dockerfile" {
  filename = "${path.module}/.coderv2/dockerfile/workspace/Dockerfile"
  content = <<EOF
    FROM ${var.workspace_image}
    USER root
    RUN openssl s_client -connect ${local.coder_address}:443 -servername ${local.coder_url} </dev/null 2>/dev/null |\
        sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p' | tee /usr/local/share/ca-certificates/coder.crt && \
        update-ca-certificates
    USER coder
  EOF
}

resource "docker_image" "workspace" {
  name = local.rebuilt_workspace_image
  build {
    context = dirname(abspath(local_file.workspace_dockerfile.filename))
  }
}

resource "null_resource" "push_workspace_image" {
  depends_on = [ docker_image.workspace ]
  provisioner "local-exec" {
    command = "docker push ${local.rebuilt_workspace_image}"
  }
}

resource "local_file" "kubernetes_template" {
  filename = "${path.module}/.coderv2/templates/kubernetes/main.tf"
  content = <<EOF
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
        namespace = "${kubernetes_namespace.coder_namespace.metadata.0.name}"
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
          image             = "${local.rebuilt_workspace_image}"
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
              "cpu"    = "1"
              "memory" = "1Gi"
            }
            limits = {
              "cpu"    = "1"
              "memory" = "1Gi"
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
                  values = ["${google_container_node_pool.workspaces.name}"]
                }
              }
            }
          }
          pod_affinity {
            preferred_during_scheduling_ignored_during_execution {
              weight = 1
              pod_affinity_term {
                topology_key = "kubernetes.io/hostname"
                label_selector {
                  match_expressions {
                    key      = "app.kubernetes.io/name"
                    operator = "In"
                    values   = ["coder-workspace"]
                  }
                }
              }
            }
          }
        }
      }
    }
  EOF
}
