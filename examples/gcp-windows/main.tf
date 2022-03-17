terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
  }
}

variable "gcp_credentials" {
  sensitive = true
}

variable "gcp_project" {
  description = "The Google Cloud project to manage resources in."
}

variable "gcp_region" {
  default = "us-central1"
}

provider "google" {
  project = var.gcp_project
  region = var.gcp_region
  credentials = var.gcp_credentials
}

data "coder_workspace" "me" {
}

data "coder_agent_script" "dev" {
  arch = "amd64"
  os   = "windows"
}

data "google_compute_default_service_account" "default" {
}

resource "random_string" "random" {
  count = data.coder_workspace.me.transition == "start" ? 1 : 0
  length  = 8
  special = false
}

resource "google_compute_instance" "dev" {
  zone         = "us-central1-a"
  count        = data.coder_workspace.me.transition == "start" ? 1 : 0
  name         = "coder-${lower(random_string.random[0].result)}"
  machine_type = "e2-medium"
  network_interface {
    network = "default"
    access_config {
      // Ephemeral public IP
    }
  }
  boot_disk {
    initialize_params {
      image = "projects/windows-cloud/global/images/windows-server-2022-dc-core-v20220215"
    }
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  metadata = {
    windows-startup-script-ps1 = data.coder_agent_script.dev.value
    serial-port-enable = "TRUE"
  }
}

resource "coder_agent" "dev" {
  count = length(google_compute_instance.dev)
  auth {
    type        = "google-instance-identity"
    instance_id = google_compute_instance.dev[0].instance_id
  }
}
