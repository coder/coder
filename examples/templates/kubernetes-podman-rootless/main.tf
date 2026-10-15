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

variable "namespace" {
  type        = string
  description = "The Kubernetes namespace to create workspace pods in."
  default     = "coder"
}

variable "node_selector" {
  type        = map(string)
  description = "Node labels for nodes with user namespaces enabled, for example an EKS Auto Mode NodePool with advancedCompute."
  default     = {}
}

provider "kubernetes" {
  # Authenticates with the service account of the Coder provisioner.
  config_path = null
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
  startup_script = <<-EOT
    #!/bin/bash
    # Use native kernel overlay storage. Some Podman images pin
    # mount_program to fuse-overlayfs in /etc/containers/storage.conf.
    mkdir -p ~/.config/containers
    printf '[storage]\ndriver = "overlay"\n\n[storage.options.overlay]\nmount_program = ""\n' > ~/.config/containers/storage.conf
    # Docker-compatible API socket for Testcontainers, Compose, and
    # the dev container docker-outside-of-docker feature.
    (podman system service --time=0 unix:///tmp/podman.sock >/dev/null 2>&1 &)
    for i in $(seq 50); do [ -S /tmp/podman.sock ] && break; sleep 0.1; done
  EOT

  env = {
    DOCKER_HOST = "unix:///tmp/podman.sock"
    # BuildKit can't start inside the workspace; use the classic builder
    # for docker build and docker compose build.
    DOCKER_BUILDKIT = "0"
  }
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}"
    namespace = var.namespace
    annotations = {
      # Allow Podman to create nested user namespaces on AppArmor hosts
      # such as GKE and Ubuntu nodes. Ignored where AppArmor is absent.
      "container.apparmor.security.beta.kubernetes.io/dev" = "unconfined"
    }
  }
  spec {
    node_selector = var.node_selector
    security_context {
      run_as_user     = 1000
      fs_group        = 1000
      run_as_non_root = true
    }
    container {
      name    = "dev"
      image   = "codercom/example-podman:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      resources {
        requests = {
          cpu    = "500m"
          memory = "1Gi"
        }
        limits = {
          cpu    = "2"
          memory = "4Gi"
        }
      }
      volume_mount {
        name       = "container-storage"
        mount_path = "/home/coder/.local/share/containers"
      }
    }
    volume {
      name = "container-storage"
      empty_dir {
        size_limit = "20Gi"
      }
    }
  }
}
