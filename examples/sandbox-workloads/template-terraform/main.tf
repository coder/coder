# The same warm pool, claimed by Terraform in an ordinary template. A
# workspace of this template is built by the Terraform provisioner like any
# other. Its daemon still proves which pod it is with the kubelet's token:
# the pod's UID is registered as the agent's instance ID.
#
# It starts in seconds, not in under a second. See ../DESIGN.md.
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
  description = "Namespace of the warm pool."
  default     = "coder-sandbox-workloads"
}

variable "warm_pool" {
  type        = string
  description = "The SandboxWarmPool to claim a pod from."
  default     = "coder-workload"
}

data "coder_workspace" "me" {}

locals {
  claim = "coder-${data.coder_workspace.me.id}"
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  # The provider has no "kubernetes" authentication type yet. Any instance
  # identity makes coderd find the daemon by its instance ID, which is what
  # the daemon in the pod asks for.
  auth = "google-instance-identity"
}

resource "kubernetes_manifest" "claim" {
  count = data.coder_workspace.me.start_count
  manifest = {
    apiVersion = "extensions.agents.x-k8s.io/v1beta1"
    kind       = "SandboxClaim"
    metadata = {
      name      = local.claim
      namespace = var.namespace
      # Naming the agent here is what makes it the agent of this resource.
      annotations = {
        "coder.com/agent-id" = coder_agent.main.id
      }
    }
    spec = {
      warmPoolRef = { name = var.warm_pool }
    }
  }
  wait {
    condition {
      type   = "Ready"
      status = "True"
    }
  }
}

# Follow the claim to its pod.
data "kubernetes_resource" "claim" {
  count       = data.coder_workspace.me.start_count
  api_version = "extensions.agents.x-k8s.io/v1beta1"
  kind        = "SandboxClaim"
  metadata {
    name      = local.claim
    namespace = var.namespace
  }
  depends_on = [kubernetes_manifest.claim]
}

data "kubernetes_pod_v1" "claimed" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = data.kubernetes_resource.claim[0].object.status.sandbox.name
    namespace = var.namespace
  }
}

# The daemon in the pod is the daemon of this workspace because the cluster
# says it runs in this pod.
resource "coder_agent_instance" "pod" {
  count       = data.coder_workspace.me.start_count
  agent_id    = coder_agent.main.id
  instance_id = "k8s-pod:${data.kubernetes_pod_v1.claimed[0].metadata[0].uid}"
}
