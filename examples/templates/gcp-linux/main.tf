terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 4.34.0"
    }
  }
}

data "coder_parameter" "project_id" {
  name = "Project ID"
  icon = "/icon/folder.svg"
  description = "Which Google Compute Project should your workspace live in?"
  default = "something"
}

data "coder_parameter" "region" {
  name = "Region"
  description = "Select a location for your workspace to live."
  icon = "/emojis/1f30e.png"
  option {
    name = "Toronto, Canada"
    value = "northamerica-northeast1-a"
    icon = "/emojis/1f1e8-1f1e6.png"
  }
  option {
    name = "Council Bluff, Iowa, USA"
    value = "us-central1-a"
    icon = "/emojis/1f920.png"
  }
  option {
    name = "Hamina, Finland"
    value = "europe-north1-a"
    icon = "/emojis/1f1eb-1f1ee.png"
  }
  option {
    name = "Warsaw, Poland"
    value = "europe-central2-a"
    icon = "/emojis/1f1f5-1f1f1.png"
  }
  option {
    name = "Madrid, Spain"
    value = "europe-southwest1-a"
    icon = "/emojis/1f1ea-1f1f8.png"
  }
  option {
    name = "London, England"
    value = "europe-west2-a"
    icon = "/emojis/1f1ec-1f1e7.png"
  }
}

provider "google" {
  zone    = data.coder_parameter.region.value
  project = data.coder_parameter.project_id.value
}

data "google_compute_default_service_account" "default" {
}

data "coder_workspace" "me" {
}

resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.id}-root"
  type  = "pd-ssd"
  zone  = data.coder_parameter.region.value
  image = "debian-cloud/debian-11"
  lifecycle {
    ignore_changes = [name, image]
  }
}

resource "coder_agent" "main" {
  auth           = "google-instance-identity"
  arch           = "amd64"
  os             = "linux"
  startup_script = <<EOT
    #!/bin/bash

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log
    code-server --auth none --port 13337 | tee code-server-install.log &
  EOT
}

# code-server
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  icon         = "/icon/code.svg"
  url          = "http://localhost:13337?folder=/home/coder"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}

resource "google_compute_instance" "dev" {
  zone         = data.coder_parameter.region.value
  count        = data.coder_workspace.me.start_count
  name         = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}-root"
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
  # The startup script runs as root with no $HOME environment set up, so instead of directly
  # running the agent init script, create a user (with a homedir, default shell and sudo
  # permissions) and execute the init script as that user.
  metadata_startup_script = <<EOMETA
#!/usr/bin/env sh
set -eux

# If user does not exist, create it and set up passwordless sudo
if ! id -u "${local.linux_user}" >/dev/null 2>&1; then
  useradd -m -s /bin/bash "${local.linux_user}"
  echo "${local.linux_user} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-user
fi

exec sudo -u "${local.linux_user}" sh -c '${coder_agent.main.init_script}'
EOMETA
}

locals {
  # Ensure Coder username is a valid Linux username
  linux_user = lower(substr(data.coder_workspace.me.owner, 0, 32))
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
