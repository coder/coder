terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.20"
    }

    kubectl = {
      source  = "alekc/kubectl"
      version = ">= 2.0.0"
    }

    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.9"
    }

    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }

    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  required_version = "~> 1.9.0"
}

provider "kubernetes" {
  config_path = var.kubernetes_kubeconfig_path
}

provider "kubectl" {
  config_path = var.kubernetes_kubeconfig_path
}

provider "helm" {
  kubernetes {
    config_path = var.kubernetes_kubeconfig_path
  }
}
