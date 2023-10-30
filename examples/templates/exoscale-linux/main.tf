terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.12.2"
    }
    exoscale = {
      source  = "exoscale/exoscale"
      version = "0.52.1"
    }
  }
}

provider "exoscale" {
  key    = var.exoscale_api_key == "" ? null : var.exoscale_api_key
  secret = var.exoscale_api_secret == "" ? null : var.exoscale_api_secret
}

variable "exoscale_api_key" {
  description = "Exoscale API Key"
  sensitive   = true
  default     = ""
}

variable "exoscale_api_secret" {
  description = "Exoscale API Secret"
  sensitive   = true
  default     = ""
}


resource "exoscale_security_group" "security_group" {
  name = "${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-sg"
}

resource "exoscale_security_group_rule" "ssh" {
  security_group_id = exoscale_security_group.security_group.id
  type              = "INGRESS"
  protocol          = "TCP"
  cidr              = "0.0.0.0/0" # "::/0" for IPv6
  start_port        = 22
  end_port          = 22
}

resource "exoscale_security_group_rule" "tailnet" {
  security_group_id = exoscale_security_group.security_group.id
  type              = "INGRESS"
  protocol          = "UDP"
  cidr              = "0.0.0.0/0"
  start_port        = 1024
  end_port          = 65355
}

data "exoscale_compute_template" "vm_template" {
  zone = module.exoscale-zone.value
  name = "Linux Ubuntu 22.04 LTS 64-bit"
}

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
cloud_final_modules:
- [scripts-user, always]
hostname: ${lower(data.coder_workspace.me.name)}
users:
- name: coder
  sudo: ALL=(ALL) NOPASSWD:ALL
  shell: /bin/bash

--//
Content-Type: text/x-shellscript; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="userdata.txt"

#!/bin/bash

echo "CODER_AGENT_TOKEN=${coder_agent.dev.token}" >> /etc/environment
echo "HOME=/home/coder" >> /etc/environment
echo "export HOME=/home/coder" >> /home/coder/.bashrc
mkdir /home/coder/workspace
chown coder: /home/coder/workspace

sudo -E -u coder /bin/bash -c '${coder_agent.dev.init_script}'
--//--
EOT
}

resource "exoscale_compute_instance" "instance" {
  zone  = module.exoscale-zone.value
  count = data.coder_workspace.me.transition == "start" ? 1 : 0
  name  = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"

  template_id = data.exoscale_compute_template.vm_template.id
  type        = module.exoscale-instance-type.value
  disk_size   = 10

  security_group_ids = [
    exoscale_security_group.security_group.id
  ]

  state = data.coder_workspace.me.transition == "start" ? "running" : "stopped"

  user_data = data.coder_workspace.me.start_count > 0 ? local.user_data : ""

  labels = {
    Name              = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    Coder_Provisioned = "true"
  }
}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $HOME"
    interval     = 60
    timeout      = 1
  }
}

resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = exoscale_compute_instance.instance[0].id
  item {
    key   = "zone"
    value = module.exoscale-zone.value
  }
  item {
    key   = "instance type"
    value = module.exoscale-instance-type.value
  }
}

module "code-server" {
  source   = "https://registry.coder.com/modules/code-server"
  agent_id = coder_agent.dev.id
  folder   = "/home/coder/workspace"
}

module "exoscale-zone" {
  source  = "https://registry.coder.com/modules/exoscale-zone"
  default = "at-vie-1"
}

module "exoscale-instance-type" {
  source  = "https://registry.coder.com/modules/exoscale-instance-type"
  default = "standard.medium"
  exclude = [
    "standard.tiny",
    "standard.micro",
    "standard.small",
    "standard.huge",
    "standard.mega",
    "standard.titan",
    "standard.jumbo",
    "standard.colossus",
  ]
}
