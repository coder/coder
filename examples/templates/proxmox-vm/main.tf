terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
    proxmox = {
      source  = "telmate/proxmox"
      version = "2.9.10"
    }
  }
}

# code-server
resource "coder_app" "code-server" {
  agent_id      = coder_agent.dev.id
  name          = "code-server"
  icon          = "https://upload.wikimedia.org/wikipedia/commons/thumb/9/9a/Visual_Studio_Code_1.35_icon.svg/2560px-Visual_Studio_Code_1.35_icon.svg.png"
  url           = "http://localhost:13337"
  relative_path = true
}

data "coder_workspace" "me" {
}

resource "coder_agent" "dev" {
  arch           = "amd64"
  auth           = "token"
  dir            = "/home/${lower(data.coder_workspace.me.owner)}"
  os             = "linux"
  startup_script = <<EOT
#!/bin/sh
curl -fsSL https://code-server.dev/install.sh | sh
code-server --auth none --port 13337 &
  EOT
}

variable "proxmox_api_auth_info" {
  description = <<EOF
  Coder will try to authenticate to Proxmox with PM_API_
  environment variables on the Coder server. See:
  https://registry.terraform.io/providers/Telmate/proxmox/latest/docs

  To authenticate with another method, edit this template (`main.tf`)

  Press ENTER to continue
  EOF

  sensitive = true
}

variable "proxmox_api_url" {
  description = <<EOF
  Proxmox API URL. Often ends in /api2/json

  If you specified PM_API_URL, press ENTER to skip
  EOF
  sensitive   = true
}

variable "proxmox_api_insecure" {
  description = <<EOF
  Select "true" if you have an invalid TLS certificate
  
  Disable TLS verification?
  EOF

  validation {
    condition = contains([
      "true",
      "false"
    ], var.proxmox_api_insecure)
    error_message = "Specify true or false."
  }
  sensitive = true
}

variable "vm_target_node" {
  description = "VM target node (often \"proxmox\")"
  sensitive   = true
}

variable "vm_cloudinit_ipconfig0" {
  description = <<EOF
  ipconfig0 for VM
  
  e.g `ip=dhcp` or `ip=10.0.2.99/16,gw=10.0.2.2`
  see: https://registry.terraform.io/providers/Telmate/proxmox/latest/docs/resources/vm_qemu#top-level-block
  EOF

  sensitive = true
}

provider "proxmox" {
  pm_api_url      = var.proxmox_api_url != "" ? var.proxmox_api_url : null
  pm_tls_insecure = tobool(var.proxmox_api_insecure)

  # We recommend authenticating via
  # environment variables, but you can
  # also specify variables here :)

  # For debugging Terraform provider errors:
  # pm_log_enable = true
  # pm_log_file   = "/var/log/terraform-plugin-proxmox.log"
  # pm_debug      = true
  # pm_log_levels = {
  #   _default    = "debug"
  #   _capturelog = ""
  # }
}

# Cloud-init data for VM to auto-start Coder
locals {
  user_data = <<EOT
Content-Type: multipart/mixed; boundary="//"
MIME-Version: 1.0

--//
Content-Type: text/cloud-config; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="cloud-config.txt"

#cloud-config
hostname: ${lower(data.coder_workspace.me.name)}
users:
- name: ${lower(data.coder_workspace.me.owner)}
  sudo: ALL=(ALL) NOPASSWD:ALL
  shell: /bin/bash
cloud_final_modules:
- [scripts-user, always]

--//
Content-Type: text/x-shellscript; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="userdata.txt"

#!/bin/bash
export CODER_AGENT_TOKEN=${coder_agent.dev.token}
sudo --preserve-env=CODER_AGENT_TOKEN -u ${lower(data.coder_workspace.me.owner)} /bin/bash -c '${coder_agent.dev.init_script}'
--//--
EOT
}

# Copy the generated cloud_init config to the Proxmox node
resource "null_resource" "cloud_init_config_files" {
  count = 1
  connection {
    type        = "ssh"
    user        = "root"
    host        = "proxmox"
    private_key = file("/root/.ssh/id_rsa")
  }

  provisioner "remote-exec" {
    inline = [<<EOT
cat << 'EOF' > "/var/lib/vz/snippets/user_data_vm-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}.yml"
${local.user_data}
EOF
    EOT
    ]

  }
}

variable "clone_template" {
  default     = "ubuntu-2004-cloudinit-template"
  description = <<EOF
  Coder requires a cloud-init compatible Proxmox template.

  (You can edit this template to specify more images and remove this message)

  See the docs:
  https://github.com/coder/coder/tree/main/examples/templates/proxmox-vm

  Follow this guide to add an Ubuntu 20.04 cloud-init template:
  https://austinsnerdythings.com/2021/08/30/how-to-create-a-proxmox-ubuntu-cloud-init-image/

  Specify the name of the VM template to clone:
  EOF

  validation {
    condition     = contains(["ubuntu-2004-cloudinit-template"], var.clone_template)
    error_message = "Invalid clone_template."
  }
}

variable "disk_size" {
  default = 10
  validation {
    condition = (
      var.disk_size >= 5 &&
      var.disk_size <= 100
    )
    error_message = "Disk size must be integer between 5 and 100 (GB)."
  }
}

variable "cpu_cores" {
  default = 1
  validation {
    condition = (
      var.cpu_cores >= 1 &&
      var.cpu_cores <= 8
    )
    error_message = "CPU cores must be integer between 1 and 8."
  }
}

variable "sockets" {
  default = 1
  validation {
    condition = (
      var.sockets >= 1 &&
      var.sockets <= 2
    )
    error_message = "Sockets must be integer between 1 and 2."
  }
}

variable "memory" {
  default = "2048"
  validation {
    condition = contains([
      "1024",
      "2048",
      "3072",
      "4098",
      "6144"
    ], var.memory)
    error_message = "Invalid memory value."
  }
}


# Provision the proxmox VM
resource "proxmox_vm_qemu" "cloudinit-test" {
  count = 1
  depends_on = [
    null_resource.cloud_init_config_files,
  ]

  name        = replace("${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}", " ", "_")
  target_node = var.vm_target_node

  # Image to clone
  clone = var.clone_template

  cores   = parseint(var.cpu_cores, 10)
  sockets = parseint(var.sockets, 10)
  memory  = parseint(var.memory, 10)
  cpu     = "kvm64"
  disk {
    slot    = 0
    size    = "${parseint(var.disk_size, 10)}G"
    type    = "scsi"
    storage = "local-lvm"
  }
  nic      = "virtio"
  bootdisk = "scsi0"
  bridge   = "vmbr0"
  agent    = 1

  os_type   = "cloud-init"
  ipconfig0 = var.vm_cloudinit_ipconfig0


  # Mount the custom cloud init config we copied to the Proxmox node
  cicustom                = "user=local:snippets/user_data_vm-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}.yml"
  cloudinit_cdrom_storage = "local-lvm"

  # Use these for debugging workspaces you cannot SSH into
  # ssshkeys = [""]

}
