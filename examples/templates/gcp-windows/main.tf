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

provider "coder" {}

variable "project_id" {
  description = "Which Google Compute Project should your workspace live in?"
}

# See https://registry.coder.com/modules/gcp-region
module "gcp_region" {
  source = "registry.coder.com/modules/gcp-region/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  regions = ["us", "europe"]
  default = "us-central1-a"
}

provider "google" {
  zone    = module.gcp_region.value
  project = var.project_id
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "google_compute_default_service_account" "default" {}

resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.id}-root"
  type  = "pd-ssd"
  zone  = module.gcp_region.value
  image = "projects/windows-cloud/global/images/windows-server-2022-dc-core-v20220215"
  lifecycle {
    ignore_changes = [name, image]
  }
}

resource "coder_agent" "main" {
  auth = "google-instance-identity"
  arch = "amd64"
  os   = "windows"
}

resource "google_compute_instance" "dev" {
  zone         = module.gcp_region.value
  count        = data.coder_workspace.me.start_count
  name         = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}"
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
    windows-startup-script-ps1 = coder_agent.main.init_script
    serial-port-enable         = "TRUE"
  }
}
resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = google_compute_instance.dev[0].id

  item {
    key   = "type"
    value = google_compute_instance.dev[0].machine_type
  }
}

resource "coder_metadata" "home_info" {
  resource_id = google_compute_disk.root.id

  item {
    key   = "size"
    value = "${google_compute_disk.root.size} GiB"
  }
}
