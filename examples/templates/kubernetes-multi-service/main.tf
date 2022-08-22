terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.9"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.10"
    }
  }
}

variable "use_kubeconfig" {
  type        = bool
  sensitive   = true
  description = <<-EOF
  Use host kubeconfig? (true/false)

  Set this to false if the Coder host is itself running as a Pod on the same
  Kubernetes cluster as you are deploying workspaces to.

  Set this to true if the Coder host is running outside the Kubernetes cluster
  for workspaces.  A valid "~/.kube/config" must be present on the Coder host.
  EOF
}

variable "workspaces_namespace" {
  type        = string
  sensitive   = true
  description = "The namespace to create workspaces in (must exist prior to creating workspaces)"
  default     = "coder-workspaces"
}

variable "disk_size" {
  type = number
  description = "Disk size (__ GB)"
  default     = 10
}

provider "kubernetes" {
  # Authenticate via ~/.kube/config or a Coder-specific ServiceAccount, depending on admin preferences
  config_path = var.use_kubeconfig == true ? "~/.kube/config" : null
}

data "coder_workspace" "me" {}

resource "coder_agent" "go" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "java" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "ubuntu" {
  os   = "linux"
  arch = "amd64"
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
  }
  spec {
    container {
      name    = "go"
      image   = "codercom/enterprise-golang:ubuntu"
      command = ["sh", "-c", coder_agent.go.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.go.token
      }
      volume_mount {
        mount_path = "/home/coder"
        name       = "go-home-directory"
      }
    }
    volume {
      name = "go-home-directory"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.go-home-directory.metadata.0.name
      }
    }
    container {
      name    = "java"
      image   = "codercom/enterprise-java:ubuntu"
      command = ["sh", "-c", coder_agent.java.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.java.token
      }
      volume_mount {
        mount_path = "/home/coder"
        name       = "java-home-directory"
      }
    }
    volume {
      name = "java-home-directory"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.java-home-directory.metadata.0.name
      }
    }
    container {
      name    = "ubuntu"
      image   = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.ubuntu.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.ubuntu.token
      }
      volume_mount {
        mount_path = "/home/coder"
        name       = "ubuntu-home-directory"
      }
    }
    volume {
      name = "ubuntu-home-directory"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.ubuntu-home-directory.metadata.0.name
      }
    }

  }
}

resource "kubernetes_persistent_volume_claim" "go-home-directory" {
  metadata {
    name      = "home-coder-go-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${var.disk_size}Gi"
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "java-home-directory" {
  metadata {
    name      = "home-coder-java-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${var.disk_size}Gi"
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "ubuntu-home-directory" {
  metadata {
    name      = "home-coder-ubuntu-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    namespace = var.workspaces_namespace
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${var.disk_size}Gi"
      }
    }
  }
}
