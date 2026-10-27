terraform {
  required_version = ">= 1.9.0, < 2.0.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "= 2.19.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "= 2.38.0"
    }
  }
}

provider "coder" {}
provider "kubernetes" {
  config_context = var.kubeconfig_context
}

variable "kubeconfig_context" {
  type        = string
  description = "Explicit context in the provisioner's local KUBE_CONFIG_PATH."
  default     = "default"
}

variable "namespace" {
  type        = string
  description = "Existing namespace for workspace pods, storage, and network policies."
  default     = "workspaces"
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os                      = "linux"
  arch                    = "amd64"
  dir                     = "/workspace/serpent"
  startup_script          = file("${path.module}/startup.sh")
  startup_script_behavior = "blocking"
  env = {
    HOME                = "/root"
    USER                = "root"
    SHELL               = "/bin/bash"
    PATH                = "/root/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    GOPROXY             = "https://proxy.golang.org"
    GOSUMDB             = "sum.golang.org"
    GOTOOLCHAIN         = "local"
    GOPATH              = "/root/go"
    GOCACHE             = "/workspace/cache/go-build"
    GOLANGCI_LINT_CACHE = "/workspace/cache/golangci-lint"
    GOFLAGS             = "-p=2"
    GOMAXPROCS          = "2"
    GOMEMLIMIT          = "768MiB"
  }
}

resource "kubernetes_persistent_volume_claim_v1" "workspace" {
  wait_until_bound = false
  metadata {
    name      = "serpent-${data.coder_workspace.me.id}"
    namespace = var.namespace
    labels = {
      "app.kubernetes.io/name" = "serpent-dev"
      "coder.template_id"      = data.coder_workspace.me.template_id
      "coder.workspace_id"     = data.coder_workspace.me.id
    }
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = { storage = "10Gi" }
    }
  }
}

resource "kubernetes_pod_v1" "workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
    namespace = var.namespace
    labels = {
      "app.kubernetes.io/name" = "serpent-dev"
      "coder.template_id"      = data.coder_workspace.me.template_id
      "coder.workspace_id"     = data.coder_workspace.me.id
    }
  }
  spec {
    automount_service_account_token = false
    container {
      name              = "workspace"
      image             = "golang:1.21.13-bookworm@sha256:c6a5b9308b3f3095e8fde83c8bf4d68bd101fce606c1a0a1394522542509dda9"
      image_pull_policy = "IfNotPresent"
      working_dir       = "/root"
      command           = ["sh", "-c", coder_agent.main.init_script]
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      security_context {
        run_as_user                = 0
        allow_privilege_escalation = false
        capabilities {
          drop = ["ALL"]
        }
      }
      resources {
        requests = { cpu = "250m", memory = "512Mi" }
        limits   = { cpu = "2", memory = "1536Mi" }
      }
      volume_mount {
        name       = "workspace"
        mount_path = "/workspace"
      }
    }
    volume {
      name = "workspace"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.workspace.metadata[0].name
      }
    }
  }
}

resource "coder_metadata" "workspace" {
  count       = data.coder_workspace.me.start_count
  resource_id = kubernetes_pod_v1.workspace[0].id
  icon        = "/emojis/1f40d.png"
  item {
    key   = "repository"
    value = "https://github.com/coder/serpent"
  }
  item {
    key   = "start here"
    value = "cd /workspace/serpent && make lint"
  }
  item {
    key   = "storage"
    value = "10 GiB persistent storage for source, tools, and caches."
  }
}
