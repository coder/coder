terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
  }
}

provider "coder" {}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context == "" ? null : var.kube_context
}

variable "kubeconfig_path" {
  type        = string
  description = "Kubeconfig the provisioner uses to reach the cluster."
  default     = "~/.kube/config"
}

variable "kube_context" {
  type        = string
  description = "Kubeconfig context. Empty uses the current context."
  default     = ""
}

variable "namespace" {
  type        = string
  description = "Namespace for the workspace's pod."
  default     = "coder-hosted-workloads"
}

variable "image" {
  type        = string
  description = "Image for the workspace. It must provide dockerd and the dind wrapper."
  default     = "docker:27-dind"
}

variable "workload_image" {
  type        = string
  description = "Image for workload sandboxes that do not name one."
  default     = "alpine:3.22"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  name = "coder-${data.coder_workspace.me.id}"
}

# The one workspace daemon. It serves the human, and it runs workloads in
# sandboxes: `coder run` needs no other resource.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/root"
}

resource "kubernetes_deployment_v1" "main" {
  count            = data.coder_workspace.me.start_count
  wait_for_rollout = false

  metadata {
    name      = local.name
    namespace = var.namespace
    labels = {
      "coder.com/workspace-id" = data.coder_workspace.me.id
    }
  }

  spec {
    replicas = 1
    selector {
      match_labels = {
        "app.kubernetes.io/instance" = local.name
      }
    }
    template {
      metadata {
        labels = {
          "app.kubernetes.io/instance" = local.name
        }
      }
      spec {
        automount_service_account_token = false
        container {
          name  = "dev"
          image = var.image
          # Docker runs next to the daemon, in the same container, because
          # the two share files: each sandbox reaches the daemon through a
          # socket in a directory that Docker mounts into it.
          #
          # Docker listens on its unix socket only. A TCP listener would be
          # reachable from sandboxes. --icc=false keeps sandboxes from
          # reaching each other.
          command = ["sh", "-c", <<-EOT
            dind dockerd --host=unix:///var/run/docker.sock --icc=false >/var/log/dockerd.log 2>&1 &
            until docker version >/dev/null 2>&1; do sleep 1; done
            docker pull --quiet "$CODER_AGENT_WORKLOAD_IMAGE" || true
            ${coder_agent.main.init_script}
          EOT
          ]
          # Docker needs a privileged container. The human therefore has
          # root in the workspace and can reach Docker; workloads cannot.
          security_context {
            privileged = true
          }
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          # Makes the daemon run workloads, each in a container of its own.
          env {
            name  = "CODER_AGENT_WORKLOAD_RUNTIME"
            value = "docker"
          }
          env {
            name  = "CODER_AGENT_WORKLOAD_IMAGE"
            value = var.workload_image
          }
          # Sandboxes are not devcontainers.
          env {
            name  = "CODER_AGENT_DEVCONTAINERS_ENABLE"
            value = "false"
          }
          volume_mount {
            name       = "docker"
            mount_path = "/var/lib/docker"
          }
          resources {
            requests = {
              cpu    = "250m"
              memory = "512Mi"
            }
            limits = {
              memory = "2Gi"
            }
          }
        }
        volume {
          name = "docker"
          empty_dir {}
        }
      }
    }
  }
}
