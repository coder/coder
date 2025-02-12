terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    google = {
      source = "hashicorp/google"
    }
  }
}

locals {
  name       = "matifali"
  project_id = "coder-dev-1"
  zone       = "asia-south1-a"
}

provider "random" {}

provider "google" {
  zone    = local.zone
  project = local.project_id
}

resource "random_string" "disk_name" {
  length  = 16
  special = false
  upper   = false
  numeric = false
}

resource "google_compute_disk" "example_disk" {
  name = "${local.name}disk-${random_string.disk_name.result}"
  type = "pd-standard"
  size = 3 # Disk size in GB
}

resource "coder_pool_resource_claimable" "prebuilt_disk" {
  other {
    instance_id = google_compute_disk.example_disk.id
  }
}
