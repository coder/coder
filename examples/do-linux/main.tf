terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.1"
    }
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }
}

variable "step1_do_token" {
  type        = string
  description = "Enter token (refer to docs at ...)"
  sensitive   = true

  validation {
    condition     = length(var.step1_do_token) == 71 && substr(var.step1_do_token, 0, 4) == "dop_"
    error_message = "Invalid Digital Ocean Personal Access Token."
  }
}

variable "step2_do_project_id" {
  type        = string
  description = "Enter project ID (see e.g. doctl projects list)"
  sensitive   = true

  validation {
    condition     = length(var.step2_do_project_id) == 36
    error_message = "Invalid Digital Ocean Project ID."
  }
}

variable "droplet_image" {
  description = "Which Droplet image would you like to use for your workspace?"
  default     = "ubuntu-22-04-x64"
  validation {
    condition     = contains(["debian-11-x64", "fedora-36-x64", "ubuntu-22-04-x64"], var.droplet_image)
    error_message = "Value must be debian-11-x64, fedora-36-x64 or ubuntu-22-04-x64."
  }
}

variable "droplet_size" {
  description = "Which Droplet configuration would you like to use?"
  validation {
    condition     = contains(["s-1vcpu-1gb", "s-1vcpu-2gb", "s-2vcpu-2gb"], var.droplet_size)
    error_message = "Value must be s-1vcpu-1gb, s-1vcpu-2gb or s-2vcpu-2gb."
  }
}

variable "region" {
  description = "Which region would you like to use?"
  validation {
    condition     = contains(["nyc1", "nyc3", "ams3"], var.region)
    error_message = "Value must be nyc1, nyc3, or ams3."
  }
}

# Configure the DigitalOcean Provider
provider "digitalocean" {
  token = var.step1_do_token
}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "digitalocean_volume" "home_volume" {
  region                   = var.region
  name                     = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-home"
  size                     = 20
  initial_filesystem_type  = "ext4"
  initial_filesystem_label = "coder-home"
}

resource "digitalocean_droplet" "workspace" {
  region     = var.region
  count      = data.coder_workspace.me.start_count
  name       = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
  image      = var.droplet_image
  size       = var.droplet_size
  volume_ids = [digitalocean_volume.home_volume.id]
  user_data = templatefile("cloud-config.yaml.tftpl", {
    username          = data.coder_workspace.me.owner
    home_volume_label = digitalocean_volume.home_volume.initial_filesystem_label
    init_script       = base64encode(coder_agent.dev.init_script)
    coder_agent_token = coder_agent.dev.token
  })
}

# resource "digitalocean_project_resources" "project" {
#   project = var.step2_do_project_id
#   # Workaround for terraform plan when using count.
#   resources = length(digitalocean_droplet.workspace) > 0 ? [
#     digitalocean_volume.home_volume.urn,
#     digitalocean_droplet.workspace[0].urn
#     ] : [
#     digitalocean_volume.home_volume.urn
#   ]
# }
