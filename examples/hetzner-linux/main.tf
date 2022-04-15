terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.3.1"
    }
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.33.1"
    }
  }
}

data "coder_workspace" "me" {
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
}

variable "hcloud_token" {
  description = <<EOF
Coder requires a Hetzner Cloud token to provision workspaces.
EOF
  sensitive   = true
}

variable "instance_location" {
  description = "What region should your workspace live in?"
  default     = "nbg1"
  validation {
    condition     = contains(["nbg1", "fsn1", "hel1"], var.instance_location)
    error_message = "Invalid zone!"
  }
}

variable "instance_type" {
  description = "What instance type should your workspace use?"
  default     = "cx11"
  validation {
    condition     = contains(["cx11", "cx21", "cx31", "cx41", "cx51"], var.instance_type)
    error_message = "Invalid zone!"
  }
}

variable "instance_os" {
  description = "Which operating system should your workspace use?"
  default     = "ubuntu-20.04"
  validation {
    condition     = contains(["ubuntu-20.04", "ubuntu-18.04", "debian-11", "debian-10", "fedora-35"], var.instance_os)
    error_message = "Invalid zone!"
  }
}


provider "hcloud" {
  token = var.hcloud_token
}

resource "hcloud_server" "root" {
  count = data.coder_workspace.me.start_count
  name        = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  server_type = var.instance_type
  location    = var.instance_location
  image       = var.instance_os
  user_data   = <<EOF
#!/bin/bash
export CODER_TOKEN=${coder_agent.dev.token}
${coder_agent.dev.init_script}"
EOF
}

resource "hcloud_volume" "root" {
  name         = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  size         = 50
  format       = "ext4"
  location     = var.instance_location
}

resource "hcloud_volume_attachment" "root" {
  count     = data.coder_workspace.me.start_count
  volume_id = hcloud_volume.root.id
  server_id = hcloud_server.root[count.index].id
  automount = true
}
