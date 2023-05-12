terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 4.36"
    }

    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.20"
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

    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }

  required_version = "~> 1.4.0"
}
