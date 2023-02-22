terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.5.3"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.10"
    }
  }
}

provider "kubernetes" {
  config_path = "~/.kube/config"
}

data "coder_workspace" "me" {}

variable "os" {
  description = "Operating system"
  validation {
    condition     = contains(["ubuntu", "fedora"], var.os)
    error_message = "Invalid zone!"
  }
  default = "ubuntu"
}

variable "cpus" {
  type        = number
  description = "How many CPUs would you like your workspace to have?"
  default     = 4
  validation {
    condition     = var.cpus > 0
    error_message = "Value must be greater than 0."
  }
}

variable "memory" {
  type        = number
  description = "How much memory would you like your workspace to have (in GiB)?"
  default     = 4
  validation {
    condition     = var.memory > 0
    error_message = "Value must be greater than 0."
  }
}

variable "home_disk_size" {
  type        = number
  description = "How large would you like your home volume to be (in GB)?"
  default     = 10
  validation {
    condition     = var.home_disk_size >= 1
    error_message = "Value must be greater than or equal to 1."
  }
}

resource "coder_agent" "dev" {
  os             = "linux"
  arch           = "amd64"
  dir            = "/home/podman"
  startup_script = <<EOF
    #!/bin/sh

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.8.3
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &

    # Run once to avoid unnecessary warning: "/" is not a shared mount
    podman ps
  EOF
}

# code-server
resource "coder_app" "code-server" {
  agent_id = coder_agent.dev.id
  name     = "code-server"
  icon     = "/icon/code.svg"
  url      = "http://localhost:13337"
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  depends_on = [
    kubernetes_persistent_volume_claim.home-directory
  ]
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
    namespace = "default"
    annotations = {
      # Disables apparmor, required for Debian- and Ubuntu-derived systems
      "container.apparmor.security.beta.kubernetes.io/dev" = "unconfined"
    }
  }
  spec {
    security_context {
      # Runs as the "podman" user
      run_as_user = 1000
      fs_group    = 1000
    }
    container {
      name = "dev"
      # We recommend building your own from our reference: see ./images directory
      image             = "ghcr.io/coder/podman:${var.os}"
      image_pull_policy = "Always"
      command           = ["/bin/bash", "-c", coder_agent.dev.init_script]
      security_context {
        # Runs as the "podman" user
        run_as_user = "1000"
      }
      resources {
        limits = {
          # Acquire a FUSE device, powered by smarter-device-manager
          "github.com/fuse" : 1
        }
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }
      volume_mount {
        mount_path = "/home/podman"
        name       = "home-directory"
      }
    }
    volume {
      name = "home-directory"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.home-directory.metadata.0.name
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "home-directory" {
  metadata {
    name      = "coder-pvc-${data.coder_workspace.me.id}"
    namespace = "default"
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        cpus    = "${var.cpus}"
        memory  = "${var.memory}Gi"
        storage = "${var.home_disk_size}Gi"
      }
    }
  }
}
