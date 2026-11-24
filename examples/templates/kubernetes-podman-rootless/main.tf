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

data "coder_parameter" "home_disk_size" {
  name         = "home_disk_size"
  display_name = "Home disk size"
  description  = "The size of the home disk in GB, including Podman images and build cache"
  default      = "20"
  type         = "number"
  icon         = "/emojis/1f4be.png"
  mutable      = false
  validation {
    min = 1
    max = 99999
  }
}

resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
  startup_script = <<-EOT
    #!/bin/bash
    # Seed shell dotfiles that the empty home volume hides on first start.
    [ -f ~/.bashrc ] || cp -rT /etc/skel ~ || true
    # Use native kernel overlay storage. Some Podman images pin
    # mount_program to fuse-overlayfs in /etc/containers/storage.conf.
    mkdir -p ~/.config/containers
    printf '[storage]\ndriver = "overlay"\n\n[storage.options.overlay]\nmount_program = ""\n' > ~/.config/containers/storage.conf
    # Docker-compatible API socket for Testcontainers, Compose, and
    # the dev container docker-outside-of-docker feature.
    (podman system service --time=0 unix:///tmp/podman.sock >/tmp/podman-service.log 2>&1 &)
    for i in $(seq 50); do [ -S /tmp/podman.sock ] && break; sleep 0.1; done
    if [ ! -S /tmp/podman.sock ]; then
      echo "Podman API socket didn't start:" >&2
      cat /tmp/podman-service.log >&2
      exit 1
    fi
  EOT

  env = {
    DOCKER_HOST = "unix:///tmp/podman.sock"
    # BuildKit can't start inside the workspace; use the classic builder
    # for docker build and docker compose build.
    DOCKER_BUILDKIT = "0"
  }
}

resource "kubernetes_persistent_volume_claim_v1" "home" {
  metadata {
    name      = "coder-${data.coder_workspace.me.id}-home"
    namespace = var.namespace
    labels = {
      "app.kubernetes.io/name"     = "coder-pvc"
      "app.kubernetes.io/instance" = "coder-pvc-${data.coder_workspace.me.id}"
      "app.kubernetes.io/part-of"  = "coder"
      //Coder-specific labels.
      "com.coder.resource"       = "true"
      "com.coder.workspace.id"   = data.coder_workspace.me.id
      "com.coder.workspace.name" = data.coder_workspace.me.name
      "com.coder.user.id"        = data.coder_workspace_owner.me.id
      "com.coder.user.username"  = data.coder_workspace_owner.me.name
    }
    annotations = {
      "com.coder.user.email" = data.coder_workspace_owner.me.email
    }
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${data.coder_parameter.home_disk_size.value}Gi"
      }
    }
  }
}

resource "kubernetes_pod_v1" "main" {
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
    # The codercom/example-podman image is published for amd64 only.
    node_selector = merge({ "kubernetes.io/arch" = "amd64" }, var.node_selector)
    security_context {
      run_as_user     = 1000
      fs_group        = 1000
      run_as_non_root = true
      # Skip the recursive chgrp on every start, which would rewrite the
      # subuid-owned image layers in the Podman graph root.
      fs_group_change_policy = "OnRootMismatch"
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
      # The Podman graph root lives under home on this ext4 or xfs volume,
      # so kernel overlay works and images persist across restarts.
      volume_mount {
        name       = "home"
        mount_path = "/home/coder"
      }
    }
    volume {
      name = "home"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.home.metadata[0].name
      }
    }
  }
}
