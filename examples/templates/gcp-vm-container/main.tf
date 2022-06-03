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

resource "coder_agent" "dev" {
  auth = "google-instance-identity"
  arch = "amd64"
  os   = "linux"
}

module "gce-container" {
  source  = "terraform-google-modules/container-vm/google"
  version = "3.0.0"

  container = {
    image   = "mcr.microsoft.com/vscode/devcontainers/go:1"
    command = ["sh"]
    args    = ["-c", coder_agent.dev.init_script]
    securityContext = {
      privileged : true
    }
  }
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
    initialize_params {
      image = module.gce-container.source_image
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  metadata = {
    "gce-container-declaration" = module.gce-container.metadata_value
  }
  labels = {
    container-vm = module.gce-container.vm_container_label
  }
}

resource "coder_agent_instance" "dev" {
  count       = data.coder_workspace.me.start_count
  agent_id    = coder_agent.dev.id
  instance_id = google_compute_instance.dev[0].instance_id
}
