terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 4.36"
    }

    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }

    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.20"
    }

    // We use the kubectl provider to apply Custom Resources.
    // The kubernetes provider requires the CRD is already present 
    // and would require a separate apply step beforehand.
    // https://github.com/hashicorp/terraform-provider-kubernetes/issues/1367
    kubectl = {
      source  = "alekc/kubectl"
      version = ">= 2.0.0"
    }

    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.9"
    }

    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  required_version = "~> 1.9.0"
}

provider "google" {
}

provider "kubernetes" {
  host                   = "https://${google_container_cluster.cluster[0].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster[0].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

provider "kubectl" {
  host                   = "https://${google_container_cluster.cluster[0].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster[0].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
  load_config_file       = false
}

provider "helm" {
  kubernetes {
    host                   = "https://${google_container_cluster.cluster[0].endpoint}"
    cluster_ca_certificate = base64decode(google_container_cluster.cluster[0].master_auth.0.cluster_ca_certificate)
    token                  = data.google_client_config.default.access_token
  }
}
