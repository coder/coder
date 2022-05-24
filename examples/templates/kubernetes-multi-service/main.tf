terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.3.1"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.10"
    }
  }
}

variable "step1_use_kubeconfig" {
  type        = bool
  sensitive   = true
  description = <<-EOF
  Use host kubeconfig? (true/false)

  If true, a valid "~/.kube/config" must be present on the Coder host. This
  is likely not your local machine unless you are using `coder server --dev.`

  If false, proceed for instructions creating a ServiceAccount on your existing
  Kubernetes cluster.
  EOF
}

variable "step2_cluster_host" {
  type        = string
  sensitive   = true
  description = <<-EOF
  Hint: You can use:
  $ kubectl cluster-info | grep "control plane"


  Leave blank if using ~/.kube/config (from step 1)
  EOF
}

variable "step3_certificate" {
  type        = string
  sensitive   = true
  description = <<-EOF
  Use docs at https://github.com/coder/coder/tree/main/examples/templates/kubernetes-multi-service#serviceaccount to create a ServiceAccount for Coder and grab values.

  Enter CA certificate

  Leave blank if using ~/.kube/config (from step 1)
  EOF
}

variable "step4_token" {
  type        = string
  sensitive   = true
  description = <<-EOF
  Enter token (refer to docs at https://github.com/coder/coder/tree/main/examples/templates/kubernetes-multi-service#serviceaccount)

  Leave blank if using ~/.kube/config (from step 1)
  EOF
}

variable "step5_coder_namespace" {
  type        = string
  sensitive   = true
  description = <<-EOF
  Enter namespace (refer to docs at https://github.com/coder/coder/tree/main/examples/templates/kubernetes-multi-service#serviceaccount)

  Leave blank if using ~/.kube/config (from step 1)
  EOF
}

provider "kubernetes" {
  # Authenticate via ~/.kube/config or a Coder-specific ServiceAccount, depending on admin preferences
  config_path            = var.step1_use_kubeconfig == true ? "~/.kube/config" : null
  host                   = var.step1_use_kubeconfig == false ? var.step2_cluster_host : null
  cluster_ca_certificate = var.step1_use_kubeconfig == false ? base64decode(var.step3_certificate) : null
  token                  = var.step1_use_kubeconfig == false ? base64decode(var.step4_token) : null
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
    name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
  }
  spec {
    container {
      name    = "go"
      image   = "mcr.microsoft.com/vscode/devcontainers/go:1"
      command = ["sh", "-c", coder_agent.go.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.go.token
      }
    }
    container {
      name    = "java"
      image   = "mcr.microsoft.com/vscode/devcontainers/java"
      command = ["sh", "-c", coder_agent.java.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.java.token
      }
    }
    container {
      name    = "ubuntu"
      image   = "mcr.microsoft.com/vscode/devcontainers/base:ubuntu"
      command = ["sh", "-c", coder_agent.ubuntu.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.ubuntu.token
      }
    }
  }
}
