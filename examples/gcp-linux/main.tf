terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      version = "0.2.1"
    }
  }
}

variable "service_account" {
  description = <<EOF
Coder requires a Google Cloud Service Account to provision workspaces.

1. Create a service account:
   https://console.cloud.google.com/projectselector/iam-admin/serviceaccounts/create
2. Add the roles:
   - Compute Admin
   - Service Account User
3. Click on the created key, and navigate to the "Keys" tab.
4. Click "Add key", then "Create new key".
5. Generate a JSON private key, and paste the contents in \'\' quotes below.
EOF
  sensitive   = true
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
  zone        = var.zone
  credentials = var.service_account
  project     = jsondecode(var.service_account).project_id
}

data "coder_workspace" "me" {
}

data "coder_agent_script" "dev" {
  auth = "google-instance-identity"
  arch = "amd64"
  os   = "linux"
}

data "google_compute_default_service_account" "default" {
}

resource "random_string" "random" {
  length  = 8
  special = false
}

resource "google_compute_disk" "root" {
  name  = "coder-${lower(random_string.random.result)}"
  type  = "pd-ssd"
  zone  = var.zone
  image = "debian-cloud/debian-9"
  lifecycle {
    ignore_changes = [image]
  }
}

resource "google_compute_instance" "dev" {
  zone         = var.zone
  count        = data.coder_workspace.me.transition == "start" ? 1 : 0
  name         = "coder-${lower(random_string.random.result)}"
  machine_type = "e2-medium"
  network_interface {
    network = "default"
    access_config {
      // Ephemeral public IP
    }
  }
  boot_disk {
    auto_delete = false
    source = google_compute_disk.root.name
  }
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  metadata_startup_script = data.coder_agent_script.dev.value
}

resource "coder_agent" "dev" {
  count = length(google_compute_instance.dev)
  instance_id = google_compute_instance.dev[0].instance_id
}
