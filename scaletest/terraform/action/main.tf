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

    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }

  required_version = "~> 1.9.0"
}

provider "google" {
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "kubernetes" {
  alias                  = "primary"
  host                   = "https://${google_container_cluster.cluster["primary"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["primary"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

provider "kubernetes" {
  alias                  = "europe"
  host                   = "https://${google_container_cluster.cluster["europe"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["europe"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

provider "kubernetes" {
  alias                  = "asia"
  host                   = "https://${google_container_cluster.cluster["asia"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["asia"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
}

provider "kubectl" {
  alias                  = "primary"
  host                   = "https://${google_container_cluster.cluster["primary"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["primary"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
  load_config_file       = false
}

provider "kubectl" {
  alias                  = "europe"
  host                   = "https://${google_container_cluster.cluster["europe"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["europe"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
  load_config_file       = false
}

provider "kubectl" {
  alias                  = "asia"
  host                   = "https://${google_container_cluster.cluster["asia"].endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.cluster["asia"].master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.default.access_token
  load_config_file       = false
}

provider "helm" {
  alias = "primary"
  kubernetes {
    host                   = "https://${google_container_cluster.cluster["primary"].endpoint}"
    cluster_ca_certificate = base64decode(google_container_cluster.cluster["primary"].master_auth.0.cluster_ca_certificate)
    token                  = data.google_client_config.default.access_token
  }
}

provider "helm" {
  alias = "europe"
  kubernetes {
    host                   = "https://${google_container_cluster.cluster["europe"].endpoint}"
    cluster_ca_certificate = base64decode(google_container_cluster.cluster["europe"].master_auth.0.cluster_ca_certificate)
    token                  = data.google_client_config.default.access_token
  }
}

provider "helm" {
  alias = "asia"
  kubernetes {
    host                   = "https://${google_container_cluster.cluster["asia"].endpoint}"
    cluster_ca_certificate = base64decode(google_container_cluster.cluster["asia"].master_auth.0.cluster_ca_certificate)
    token                  = data.google_client_config.default.access_token
  }
}
