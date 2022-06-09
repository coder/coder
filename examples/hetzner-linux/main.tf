terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "1.33.2"
    }
  }
}

provider "hcloud" {
  token = var.hcloud_token
}

provider "coder" {
}

variable "hcloud_token" {
  description = <<EOF
Coder requires a Hetzner Cloud token to provision workspaces.
EOF
  sensitive   = true
  validation {
    condition     = length(var.hcloud_token) == 64
    error_message = "Please provide a valid Hetzner Cloud API token."
  }
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
    error_message = "Invalid instance type!"
  }
}

variable "instance_os" {
  description = "Which operating system should your workspace use?"
  default     = "ubuntu-20.04"
  validation {
    condition     = contains(["ubuntu-22.04","ubuntu-20.04", "ubuntu-18.04", "debian-11", "debian-10"], var.instance_os)
    error_message = "Invalid OS!"
  }
}

variable "volume_size" {
  description = "How much storage space do you need?"
  default     = "50"
  validation {
    condition     = contains(["50","100","150"], var.volume_size)
    error_message = "Invalid volume size!"
  }
}

variable "code_server" {
  description = "Should Code Server be installed?"
  default     = "true"
  validation {
    condition     = contains(["true","false"], var.code_server)
    error_message = "Your answer can only be yes or no!"
  }
}

data "coder_workspace" "me" {
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
}

resource "coder_app" "code-server" {
  count         = var.code_server ? 1 : 0
  agent_id      = coder_agent.dev.id
  name          = "code-server"
  icon          = "https://cdn.icon-icons.com/icons2/2107/PNG/512/file_type_vscode_icon_130084.png"
  url           = "http://localhost:8080"
  relative_path = true
}

resource "hcloud_server" "root" {
  count       = data.coder_workspace.me.start_count
  name        = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  server_type = var.instance_type
  location    = var.instance_location
  image       = var.instance_os
  user_data   = templatefile("cloud-config.yaml.tftpl", {
    username          = data.coder_workspace.me.owner
    volume_path       = "/dev/disk/by-id/scsi-0HC_Volume_${hcloud_volume.root.id}"
    init_script       = base64encode(coder_agent.dev.init_script)
    coder_agent_token = coder_agent.dev.token
    code_server_setup = var.code_server
  })
}

resource "hcloud_volume" "root" {
  name         = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  size         = var.volume_size
  format       = "ext4"
  location     = var.instance_location
}

resource "hcloud_volume_attachment" "root" {
  count     = data.coder_workspace.me.start_count
  volume_id = hcloud_volume.root.id
  server_id = hcloud_server.root[count.index].id
  automount = false
}

resource "hcloud_firewall" "root" {
  name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
  rule {
    direction = "in"
    protocol  = "icmp"
    source_ips = [
      "0.0.0.0/0",
      "::/0"
    ]
  }
}

resource "hcloud_firewall_attachment" "root_fw_attach" {
    count = data.coder_workspace.me.start_count
    firewall_id = hcloud_firewall.root.id
    server_ids  = [hcloud_server.root[count.index].id]
}
