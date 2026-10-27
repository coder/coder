# Shared root: namespace and policy are managed once per template.
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

# Requires the companion coder provider development override.
provider "coder" {}
provider "kubernetes" {
  config_context = var.kubeconfig_context
}

variable "kubeconfig_context" {
  type        = string
  description = "Explicit context in the provisioner's local KUBE_CONFIG_PATH."
  default     = "default"
}

data "coder_template" "current" {}

resource "kubernetes_namespace_v1" "workspaces" {
  metadata {
    name = "workspaces"
  }
}

module "coder_network_policy" {
  source = "./modules/coder-network-policy"

  policy_id    = data.coder_template.current.id
  template_ids = [data.coder_template.current.id]
  namespace    = kubernetes_namespace_v1.workspaces.metadata[0].name

  allowed_https_hosts = ["github.com", "proxy.golang.org", "sum.golang.org"]
  dns_query_names     = []

  # The local Coder control plane and embedded relay share this endpoint.
  control_plane_allowances = [{
    cidrs = ["172.20.0.3/32"]
    ports = [{ port = 3100, protocol = "TCP" }]
  }]
  relay_allowances   = []
  direct_p2p_enabled = false
}
