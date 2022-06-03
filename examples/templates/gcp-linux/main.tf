terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.3.4"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 4.15"
    }
  }
}

variable "project_id" {
  description = "Which Google Compute Project should your workspace live in?"
}

variable "zone" {
  description = "What region should your workspace live in?"
  default     = "us-central1-a"
  validation {
    condition     = contains(["northamerica-northeast1-a", "us-central1-a", "us-west2-c", "europe-west4-b", "southamerica-east1-a"], var.zone)
    error_message = "Invalid zone!"
  }
}

provider "google" {
  zone    = var.zone
  project = var.project_id
}

data "google_compute_default_service_account" "default" {
}

data "coder_workspace" "me" {
}

resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  type  = "pd-ssd"
  zone  = var.zone
  image = "debian-cloud/debian-9"
  lifecycle {
    ignore_changes = [image]
  }
}

resource "coder_agent" "dev" {
  auth = "google-instance-identity"
  arch = "amd64"
  os   = "linux"
}

resource "google_compute_instance" "dev" {
  zone         = var.zone
  count        = data.coder_workspace.me.start_count
  name         = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
  machine_type = "e2-medium"
  network_interface {
    network = "default"
    access_config {
      // Ephemeral public IP
    }
  }
  boot_disk {
    auto_delete = false
    source      = google_compute_disk.root.name
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  metadata_startup_script = coder_agent.dev.init_script
}
