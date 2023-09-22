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
  }

  required_version = "~> 1.5.0"
}

provider "google" {
  region  = var.region
  project = var.project_id
}
