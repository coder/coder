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

  # Greedy metadata (3072 bytes base64 encoded is 4097 bytes).
  metadata {
    display_name = "Meta 01"
    key          = "01_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 02"
    key          = "0_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 03"
    key          = "03_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 04"
    key          = "04_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 05"
    key          = "05_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 06"
    key          = "06_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 07"
    key          = "07_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 08"
    key          = "08_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 09"
    key          = "09_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 10"
    key          = "10_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 11"
    key          = "11_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 12"
    key          = "12_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 13"
    key          = "13_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 14"
    key          = "14_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 15"
    key          = "15_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
  metadata {
    display_name = "Meta 16"
    key          = "16_meta"
    script       = "dd if=/dev/urandom bs=3072 count=1 status=none | base64"
    interval     = 1
    timeout      = 10
  }
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
      image             = "docker.io/codercom/enterprise-minimal:ubuntu"
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
          "cpu"    = "2"
          "memory" = "2Gi"
        }
        limits = {
          "cpu"    = "2"
          "memory" = "2Gi"
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
