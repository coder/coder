terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13.0"
    }
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = ">= 2.0"
    }
  }
}

module "aws_region" {
  source  = "https://registry.coder.com/modules/aws-region"
  default = "eu-west-1"
}

provider "aws" {
  region = module.aws_region.value
}

# --- Variables (template-level, set by admin) ---

variable "vpc_subnet_id" {
  type        = string
  description = "The VPC subnet to launch instances in."
}

variable "security_group_id" {
  type        = string
  description = "Security group to attach to instances. Agent dials out, so minimal inbound rules are needed."
}

variable "iam_instance_profile" {
  type        = string
  default     = ""
  description = "Optional IAM instance profile name for the EC2 instance."
}

# --- Data sources ---

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  workspace_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  linux_user     = "coder"
  hostname       = lower(data.coder_workspace.me.name)
  instance_type  = "m5d.xlarge"
  data_volume_gb = 128
  az             = data.aws_subnet.workspace.availability_zone
  repo_dir       = replace(module.git-clone.repo_dir, "/^~\\//", "/home/coder/")
}

data "aws_subnet" "workspace" {
  id = var.vpc_subnet_id
}

# --- Parameters (per-workspace, set by user) ---

# --- AMI ---

data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
  owners = ["099720109477"] # Canonical
}

# --- Cloud-init ---

data "cloudinit_config" "user_data" {
  gzip          = false
  base64_encode = false
  boundary      = "//"

  part {
    filename     = "cloud-config.yaml"
    content_type = "text/cloud-config"
    content = templatefile("${path.module}/cloud-init/cloud-config.yaml.tftpl", {
      hostname   = local.hostname
      linux_user = local.linux_user
    })
  }

  part {
    filename     = "userdata.sh"
    content_type = "text/x-shellscript"
    content = templatefile("${path.module}/cloud-init/userdata.sh.tftpl", {
      linux_user        = local.linux_user
      data_volume_id    = aws_ebs_volume.data.id
      agent_token       = coder_agent.dev.token
      agent_init_script = base64encode(coder_agent.dev.init_script)
    })
  }
}

# --- EC2 instance ---

data "aws_iam_instance_profile" "this" {
  count = var.iam_instance_profile == "" ? 0 : 1
  name  = var.iam_instance_profile
}

resource "aws_instance" "workspace" {
  ami                    = data.aws_ami.ubuntu.id
  availability_zone      = local.az
  instance_type          = local.instance_type
  subnet_id              = var.vpc_subnet_id
  vpc_security_group_ids = [var.security_group_id]
  iam_instance_profile   = try(data.aws_iam_instance_profile.this[0].name, null)

  # Enforce IMDSv2 with hop limit 1 to prevent container processes
  # from accessing instance metadata (agent token, IAM credentials).
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    volume_size           = 50
    volume_type           = "gp3"
    delete_on_termination = true
  }

  # Expose local NVMe instance store for swap.
  ephemeral_block_device {
    device_name  = "/dev/sdb"
    virtual_name = "ephemeral0"
  }

  user_data = data.cloudinit_config.user_data.rendered

  tags = {
    Name              = local.workspace_name
    Coder_Provisioned = "true"
  }

  lifecycle {
    ignore_changes = [ami, user_data]
  }
}

resource "aws_ec2_instance_state" "workspace" {
  instance_id = aws_instance.workspace.id
  state       = data.coder_workspace.me.transition == "start" ? "running" : "stopped"
}

# --- Persistent EBS data volume ---
#
# Single data volume for /home/coder. Docker lives on the root
# volume so the workspace remains accessible (via the host agent)
# even if this volume fills up.

resource "aws_ebs_volume" "data" {
  availability_zone = local.az
  size              = local.data_volume_gb
  type              = "gp3"
  throughput        = 250 # MB/s
  iops              = 3000

  tags = {
    Name              = "${local.workspace_name}-data"
    Coder_Provisioned = "true"
  }

  # Protect persistent data: never recreate this volume due to
  # attribute drift. Size, IOPS, throughput, and type changes are
  # applied in-place via ModifyVolume. Only availability_zone and
  # encryption attributes force replacement — ignore those.
  # See docs/admin/templates/extending-templates/resource-persistence.md
  lifecycle {
    ignore_changes = [availability_zone, encrypted, kms_key_id, snapshot_id]
  }
}

resource "aws_volume_attachment" "data" {
  device_name  = "/dev/sdf"
  volume_id    = aws_ebs_volume.data.id
  instance_id  = aws_instance.workspace.id
  force_detach = false
}

# --- Coder agent (host — bootstrap and monitoring) ---

resource "coder_agent" "dev" {
  arch                    = "amd64"
  os                      = "linux"
  auth                    = "token"
  dir                     = local.repo_dir
  startup_script_behavior = "blocking"
  connection_timeout      = 0

  env = {
    # Enable devcontainer detection and sub-agent injection.
    CODER_AGENT_DEVCONTAINERS_ENABLE = "true"
  }

  # IDEs connect to the devcontainer sub-agent, not the host.
  display_apps {
    vscode                 = false
    vscode_insiders        = false
    ssh_helper             = true
    port_forwarding_helper = true
  }

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
    key          = "swap"
    display_name = "Swap Usage"
    interval     = 15
    timeout      = 5
    script       = <<-EOT
      free -h | awk '/Swap/{print $3"/"$2}'
    EOT
  }
  metadata {
    key          = "data_disk"
    display_name = "Data Volume (/home/coder)"
    interval     = 60
    timeout      = 5
    script       = <<-EOT
      df -h /home/coder | awk 'NR==2{print $3"/"$2" ("$5")"}'
    EOT
  }

  resources_monitoring {
    memory {
      enabled   = true
      threshold = 80
    }
    volume {
      path      = "/home/coder"
      enabled   = true
      threshold = 90
    }
  }

  startup_script = <<-EOT
    #!/bin/bash
    set -euo pipefail

    # Authenticate GitHub CLI.
    if command -v gh &>/dev/null; then
      gh auth setup-git || true
    fi
  EOT

  shutdown_script = <<-EOT
    #!/bin/bash
    set -euo pipefail

    # Clean build artifacts to reclaim EBS space.
    cd "${local.repo_dir}" 2>/dev/null && rm -rf build/ || true

    # Prune Docker to reclaim root volume space.
    docker system prune -a -f || true
  EOT
}

# --- Devcontainer ---

resource "coder_devcontainer" "coder" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = local.repo_dir
}

module "devcontainers-cli" {
  source   = "registry.coder.com/coder/devcontainers-cli/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.dev.id
}

# --- Modules ---

module "dotfiles" {
  source   = "registry.coder.com/coder/dotfiles/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.dev.id
}

module "git-clone" {
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.dev.id
  url      = "https://github.com/coder/coder.git"
  base_dir = "/home/coder"
}

# --- Metadata & Quotas ---

# Quota cost = provisioned storage GB, consistent with the EKS
# dogfood template (coder-k8s).
resource "coder_metadata" "data_volume" {
  resource_id = aws_ebs_volume.data.id
  daily_cost  = local.data_volume_gb
  item {
    key   = "size"
    value = "${local.data_volume_gb} GB"
  }
  item {
    key   = "mount"
    value = "/home/coder"
  }
}

resource "coder_metadata" "instance_info" {
  resource_id = aws_instance.workspace.id
  item {
    key   = "instance_type"
    value = local.instance_type
  }
  item {
    key   = "region"
    value = module.aws_region.value
  }
  item {
    key   = "ami"
    value = aws_instance.workspace.ami
  }
}
