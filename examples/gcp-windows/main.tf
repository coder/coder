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
5. Generate a JSON private key, and paste the contents below.
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

data "google_compute_default_service_account" "default" {
}

resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  type  = "pd-ssd"
  zone  = var.zone
  image = "projects/windows-cloud/global/images/windows-server-2022-dc-core-v20220215"
  lifecycle {
    ignore_changes = [image]
  }
}

resource "coder_agent" "dev" {
  auth = "google-instance-identity"
  arch = "amd64"
  os   = "windows"
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
  metadata = {
    windows-startup-script-ps1 = coder_agent.dev.init_script
    serial-port-enable         = "TRUE"
  }
}
