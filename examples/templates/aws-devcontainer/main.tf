terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    aws = {
      source = "hashicorp/aws"
    }
  }
}

module "aws_region" {
  source  = "https://registry.coder.com/modules/aws-region"
  default = "us-east-1"
}

data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance type"
  description  = "What instance type should your workspace use?"
  default      = "t3.micro"
  mutable      = false
  option {
    name  = "2 vCPU, 1 GiB RAM"
    value = "t3.micro"
  }
  option {
    name  = "2 vCPU, 2 GiB RAM"
    value = "t3.small"
  }
  option {
    name  = "2 vCPU, 4 GiB RAM"
    value = "t3.medium"
  }
  option {
    name  = "2 vCPU, 8 GiB RAM"
    value = "t3.large"
  }
  option {
    name  = "4 vCPU, 16 GiB RAM"
    value = "t3.xlarge"
  }
  option {
    name  = "8 vCPU, 32 GiB RAM"
    value = "t3.2xlarge"
  }
}

provider "aws" {
  region = module.aws_region.value
}

data "coder_workspace" "me" {
}

data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
  owners = ["099720109477"] # Canonical
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
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "https://registry.coder.com/modules/code-server"
  agent_id = coder_agent.dev[0].id
}

locals {
  linux_user = "coder"
  user_data  = <<-EOT
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
  - name: ${local.linux_user}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash

  --//
  Content-Type: text/x-shellscript; charset="us-ascii"
  MIME-Version: 1.0
  Content-Transfer-Encoding: 7bit
  Content-Disposition: attachment; filename="userdata.txt"

  #!/bin/bash
  # Install Docker
  if ! command -v docker &> /dev/null
  then
    echo "Docker not found, installing..."
    curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh 2>&1 >/dev/null
    usermod -aG docker ${local.linux_user}
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
  --//--
  EOT
}

resource "aws_instance" "vm" {
  ami               = data.aws_ami.ubuntu.id
  availability_zone = "${module.aws_region.value}a"
  instance_type     = data.coder_parameter.instance_type.value
  root_block_device {
    volume_size = 30
  }

  user_data = local.user_data
  tags = {
    Name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    # Required if you are using our example policy, see template README
    Coder_Provisioned = "true"
  }
  lifecycle {
    ignore_changes = [ami]
  }
}

resource "aws_ec2_instance_state" "vm" {
  instance_id = aws_instance.vm.id
  state       = data.coder_workspace.me.transition == "start" ? "running" : "stopped"
}
