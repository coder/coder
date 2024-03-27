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

variable "kubernetes_nodepool_workspaces" {
  description = "Kubernetes nodepool for Coder workspaces"
  type        = string
  default     = "big-workspaces"
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
    name      = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}"
    namespace = "coder-big"
    labels = {
      "app.kubernetes.io/name"     = "coder-workspace"
      "app.kubernetes.io/instance" = "coder-workspace-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}"
    }
  }
  spec {
    security_context {
      run_as_user = "1000"
      fs_group    = "1000"
    }
    container {
      name              = "dev"
      image             = "docker.io/codercom/enterprise-base:ubuntu"
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
              key      = "cloud.google.com/gke-nodepool"
              operator = "In"
              values   = ["${var.kubernetes_nodepool_workspaces}"]
            }
          }
        }
      }
    }
  }
}
