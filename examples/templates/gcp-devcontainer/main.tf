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

provider "coder" {
}

variable "project_id" {
  description = "Which Google Compute Project should your workspace live in?"
}

data "coder_parameter" "zone" {
  name         = "zone"
  display_name = "Zone"
  description  = "Which zone should your workspace live in?"
  type         = "string"
  icon         = "/emojis/1f30e.png"
  default      = "us-central1-a"
  mutable      = false
  option {
    name  = "North America (Northeast)"
    value = "northamerica-northeast1-a"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "North America (Central)"
    value = "us-central1-a"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "North America (West)"
    value = "us-west2-c"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Europe (West)"
    value = "europe-west4-b"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "South America (East)"
    value = "southamerica-east1-a"
    icon  = "/emojis/1f1e7-1f1f7.png"
  }
}

provider "google" {
  zone    = data.coder_parameter.zone.value
  project = var.project_id
}

data "google_compute_default_service_account" "default" {
}

data "coder_workspace" "me" {
}
data "coder_workspace_owner" "me" {}

resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.id}-root"
  type  = "pd-ssd"
  image = "debian-cloud/debian-12"
  lifecycle {
    ignore_changes = [name, image]
  }
}

data "coder_parameter" "repo_url" {
  name         = "repo_url"
  display_name = "Repository URL"
  default      = "https://github.com/coder/envbuilder-starter-devcontainer"
  description  = "Repository URL"
  mutable      = true
}

resource "coder_agent" "dev" {
  count              = data.coder_workspace.me.start_count
  arch               = "amd64"
  auth               = "token"
  os                 = "linux"
  dir                = "/workspaces/${trimsuffix(basename(data.coder_parameter.repo_url.value), ".git")}"
  connection_timeout = 0

  metadata {
    key          = "cpu"
    display_name = "CPU Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat cpu"
  }
  metadata {
    key          = "memory"
    display_name = "Memory Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat mem"
  }
  metadata {
    key          = "disk"
    display_name = "Disk Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat disk"
  }
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "https://registry.coder.com/modules/code-server"
  agent_id = coder_agent.dev[0].id
}

resource "google_compute_instance" "vm" {
  name         = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}-root"
  machine_type = "e2-medium"
  # data.coder_workspace_owner.me.name == "default"  is a workaround to suppress error in the terraform plan phase while creating a new workspace.
  desired_status = (data.coder_workspace_owner.me.name == "default" || data.coder_workspace.me.start_count == 1) ? "RUNNING" : "TERMINATED"

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
    # The startup script runs as root with no $HOME environment set up, so instead of directly
    # running the agent init script, create a user (with a homedir, default shell and sudo
    # permissions) and execute the init script as that user.
    startup-script = <<-META
    #!/usr/bin/env sh
    set -eux

    # If user does not exist, create it and set up passwordless sudo
    if ! id -u "${local.linux_user}" >/dev/null 2>&1; then
      useradd -m -s /bin/bash "${local.linux_user}"
      echo "${local.linux_user} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-user
    fi

    # Check for Docker, install if not present
    if ! command -v docker &> /dev/null
    then
      echo "Docker not found, installing..."
      curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh 2>&1 >/dev/null
      sudo usermod -aG docker ${local.linux_user}
      newgrp docker
    else
      echo "Docker is already installed."
    fi
    # Start envbuilder
    docker run --rm \
      -h ${lower(data.coder_workspace.me.name)} \
      -v /home/${local.linux_user}/envbuilder:/workspaces \
      -e CODER_AGENT_TOKEN="${try(coder_agent.dev[0].token, "")}" \
      -e CODER_AGENT_URL="${data.coder_workspace.me.access_url}" \
      -e GIT_URL="${data.coder_parameter.repo_url.value}" \
      -e INIT_SCRIPT="echo ${base64encode(try(coder_agent.dev[0].init_script, ""))} | base64 -d | sh" \
      -e FALLBACK_IMAGE="codercom/enterprise-base:ubuntu" \
      ghcr.io/coder/envbuilder
    META
  }
}

locals {
  # Ensure Coder username is a valid Linux username
  linux_user = lower(substr(data.coder_workspace_owner.me.name, 0, 32))
}

resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = google_compute_instance.vm.id

  item {
    key   = "type"
    value = google_compute_instance.vm.machine_type
  }

  item {
    key   = "zone"
    value = data.coder_parameter.zone.value
  }
}

resource "coder_metadata" "home_info" {
  resource_id = google_compute_disk.root.id

  item {
    key   = "size"
    value = "${google_compute_disk.root.size} GiB"
  }
}
