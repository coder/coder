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
  description = "Namespace for the workspace's pods. The workload operator must run in it."
  default     = "coder-runtime-workloads"
}

variable "image" {
  type        = string
  description = "Image for both daemons. The workspace agent needs /etc/os-release and curl or wget."
  default     = "alpine@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  name = "coder-${data.coder_workspace.me.id}"
}

# The workspace agent looks up its own user, and the stock image has no entry
# for the unprivileged UID the pods run as. Both pod definitions mount this
# file as /etc/passwd.
resource "kubernetes_config_map_v1" "passwd" {
  count = data.coder_workspace.me.start_count

  metadata {
    name      = "${local.name}-passwd"
    namespace = var.namespace
  }

  data = {
    passwd = <<-EOT
      root:x:0:0:root:/root:/bin/sh
      coder:x:1000:1000:coder:/home/coder:/bin/sh
    EOT
  }
}

# The daemon the human uses. Nothing about it is new: it holds a Coder agent
# token like every workspace daemon today.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
}

# The shape of the daemons that run workloads. It is declared like any other
# agent, but no pod ever runs it. `coder run` clones it at runtime.
resource "coder_agent" "workload" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"

  env = {
    # Marks this agent as a shape.
    CODER_WORKLOAD_SHAPE = "true"
  }

  # Every workload cloned from the shape runs the shape's startup script.
  startup_script = "echo \"shape startup script ran in pod $(hostname)\""
}

resource "kubernetes_deployment_v1" "main" {
  count            = data.coder_workspace.me.start_count
  wait_for_rollout = false

  metadata {
    name      = "${local.name}-main"
    namespace = var.namespace
    labels = {
      "coder.com/workspace-id" = data.coder_workspace.me.id
    }
  }

  spec {
    replicas = 1
    selector {
      match_labels = {
        "app.kubernetes.io/instance" = "${local.name}-main"
      }
    }
    template {
      metadata {
        labels = {
          "app.kubernetes.io/instance" = "${local.name}-main"
        }
      }
      spec {
        automount_service_account_token = false
        security_context {
          run_as_user  = 1000
          run_as_group = 1000
          fs_group     = 1000
        }
        container {
          name    = "dev"
          image   = var.image
          command = ["sh", "-c", coder_agent.main.init_script]
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          env {
            name  = "HOME"
            value = "/home/coder"
          }
          volume_mount {
            name       = "home"
            mount_path = "/home/coder"
          }
          volume_mount {
            name       = "passwd"
            mount_path = "/etc/passwd"
            sub_path   = "passwd"
            read_only  = true
          }
          resources {
            requests = {
              cpu    = "50m"
              memory = "128Mi"
            }
            limits = {
              memory = "512Mi"
            }
          }
        }
        volume {
          name = "home"
          empty_dir {}
        }
        volume {
          name = "passwd"
          config_map {
            name = kubernetes_config_map_v1.passwd[0].metadata[0].name
          }
        }
      }
    }
  }
}

# The shape's pod template. With zero replicas the Deployment runs nothing;
# it exists so the workload operator has a pod definition to clone. It is
# written exactly like the human's Deployment above.
resource "kubernetes_deployment_v1" "workload" {
  count            = data.coder_workspace.me.start_count
  wait_for_rollout = false

  metadata {
    name      = "${local.name}-workload"
    namespace = var.namespace
    labels = {
      "coder.com/workspace-id"   = data.coder_workspace.me.id
      "coder.com/workload-shape" = "true"
    }
  }

  spec {
    replicas = 0
    selector {
      match_labels = {
        "app.kubernetes.io/instance" = "${local.name}-workload"
      }
    }
    template {
      metadata {
        labels = {
          "app.kubernetes.io/instance" = "${local.name}-workload"
        }
      }
      spec {
        automount_service_account_token = false
        security_context {
          run_as_user  = 1000
          run_as_group = 1000
          fs_group     = 1000
        }
        container {
          name    = "dev"
          image   = var.image
          command = ["sh", "-c", coder_agent.workload.init_script]
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.workload.token
          }
          env {
            name  = "HOME"
            value = "/home/coder"
          }
          volume_mount {
            name       = "home"
            mount_path = "/home/coder"
          }
          volume_mount {
            name       = "passwd"
            mount_path = "/etc/passwd"
            sub_path   = "passwd"
            read_only  = true
          }
          resources {
            requests = {
              cpu    = "50m"
              memory = "128Mi"
            }
            limits = {
              memory = "512Mi"
            }
          }
        }
        volume {
          name = "home"
          empty_dir {}
        }
        volume {
          name = "passwd"
          config_map {
            name = kubernetes_config_map_v1.passwd[0].metadata[0].name
          }
        }
      }
    }
  }
}
