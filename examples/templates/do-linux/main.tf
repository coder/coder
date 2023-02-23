terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.6.12"
    }
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }
}

variable "step1_do_project_id" {
  type        = string
  description = <<-EOF
    Enter project ID

      $ doctl projects list
  EOF
  sensitive   = true

  validation {
    condition     = length(var.step1_do_project_id) == 36
    error_message = "Invalid Digital Ocean Project ID."
  }
}

variable "step2_do_admin_ssh_key" {
  type        = number
  description = <<-EOF
    Enter admin SSH key ID (some Droplet images require an SSH key to be set):

    Can be set to "0" for no key.

    Note: Setting this to zero will break Fedora images and notify root passwords via email.

      $ doctl compute ssh-key list
  EOF
  sensitive   = true

  validation {
    condition     = var.step2_do_admin_ssh_key >= 0
    error_message = "Invalid Digital Ocean SSH key ID, a number is required."
  }
}

variable "droplet_image" {
  type        = string
  description = "Which Droplet image would you like to use for your workspace?"
  default     = "ubuntu-22-04-x64"
  validation {
    condition     = contains(["ubuntu-22-04-x64", "ubuntu-20-04-x64", "fedora-36-x64", "fedora-35-x64", "debian-11-x64", "debian-10-x64", "centos-stream-9-x64", "centos-stream-8-x64", "rockylinux-8-x64", "rockylinux-8-4-x64"], var.droplet_image)
    error_message = "Value must be ubuntu-22-04-x64, ubuntu-20-04-x64, fedora-36-x64, fedora-35-x64, debian-11-x64, debian-10-x64, centos-stream-9-x64, centos-stream-8-x64, rockylinux-8-x64 or rockylinux-8-4-x64."
  }
}

variable "droplet_size" {
  type        = string
  description = "Which Droplet configuration would you like to use?"
  default     = "s-1vcpu-1gb"
  validation {
    condition     = contains(["s-1vcpu-1gb", "s-1vcpu-2gb", "s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "s-8vcpu-16gb"], var.droplet_size)
    error_message = "Value must be s-1vcpu-1gb, s-1vcpu-2gb, s-2vcpu-2gb, s-2vcpu-4gb, s-4vcpu-8gb or s-8vcpu-16gb."
  }
}

variable "home_volume_size" {
  type        = number
  description = "How large would you like your home volume to be (in GB)?"
  default     = 20
  validation {
    condition     = var.home_volume_size >= 1
    error_message = "Value must be greater than or equal to 1."
  }
}

variable "region" {
  type        = string
  description = "Which region would you like to use?"
  default     = "ams3"
  validation {
    condition     = contains(["nyc1", "nyc2", "nyc3", "sfo1", "sfo2", "sfo3", "ams2", "ams3", "sgp1", "lon1", "fra1", "tor1", "blr1"], var.region)
    error_message = "Value must be nyc1, nyc2, nyc3, sfo1, sfo2, sfo3, ams2, ams3, sgp1, lon1, fra1, tor1 or blr1."
  }
}

# Configure the DigitalOcean Provider
provider "digitalocean" {
  # Recommended: use environment variable DIGITALOCEAN_TOKEN with your personal access token when starting coderd
  # alternatively, you can pass the token via a variable.
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  login_before_ready = false
}

resource "digitalocean_volume" "home_volume" {
  region                   = var.region
  name                     = "coder-${data.coder_workspace.me.id}-home"
  size                     = var.home_volume_size
  initial_filesystem_type  = "ext4"
  initial_filesystem_label = "coder-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
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
    init_script       = base64encode(coder_agent.main.init_script)
    coder_agent_token = coder_agent.main.token
  })
  # Required to provision Fedora.
  ssh_keys = var.step2_do_admin_ssh_key > 0 ? [var.step2_do_admin_ssh_key] : []
}

resource "digitalocean_project_resources" "project" {
  project = var.step1_do_project_id
  # Workaround for terraform plan when using count.
  resources = length(digitalocean_droplet.workspace) > 0 ? [
    digitalocean_volume.home_volume.urn,
    digitalocean_droplet.workspace[0].urn
    ] : [
    digitalocean_volume.home_volume.urn
  ]
}

resource "coder_metadata" "workspace-info" {
  count       = data.coder_workspace.me.start_count
  resource_id = digitalocean_droplet.workspace[0].id

  item {
    key   = "region"
    value = digitalocean_droplet.workspace[0].region
  }
  item {
    key   = "image"
    value = digitalocean_droplet.workspace[0].image
  }
}

resource "coder_metadata" "volume-info" {
  resource_id = digitalocean_volume.home_volume.id

  item {
    key   = "size"
    value = "${digitalocean_volume.home_volume.size} GiB"
  }

}
