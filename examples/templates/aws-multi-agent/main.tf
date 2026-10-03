terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    aws = {
      source = "hashicorp/aws"
    }
    cloudinit = {
      source = "hashicorp/cloudinit"
    }
  }
}

# Last updated 2023-03-14
# aws ec2 describe-regions | jq -r '[.Regions[].RegionName] | sort'
data "coder_parameter" "region" {
  name         = "region"
  display_name = "Region"
  description  = "The region to deploy the workspace in."
  default      = "us-east-1"
  mutable      = false
  option {
    name  = "Asia Pacific (Tokyo)"
    value = "ap-northeast-1"
    icon  = "/emojis/1f1ef-1f1f5.png"
  }
  option {
    name  = "Asia Pacific (Seoul)"
    value = "ap-northeast-2"
    icon  = "/emojis/1f1f0-1f1f7.png"
  }
  option {
    name  = "Asia Pacific (Osaka)"
    value = "ap-northeast-3"
    icon  = "/emojis/1f1ef-1f1f5.png"
  }
  option {
    name  = "Asia Pacific (Mumbai)"
    value = "ap-south-1"
    icon  = "/emojis/1f1ee-1f1f3.png"
  }
  option {
    name  = "Asia Pacific (Singapore)"
    value = "ap-southeast-1"
    icon  = "/emojis/1f1f8-1f1ec.png"
  }
  option {
    name  = "Asia Pacific (Sydney)"
    value = "ap-southeast-2"
    icon  = "/emojis/1f1e6-1f1fa.png"
  }
  option {
    name  = "Canada (Central)"
    value = "ca-central-1"
    icon  = "/emojis/1f1e8-1f1e6.png"
  }
  option {
    name  = "EU (Frankfurt)"
    value = "eu-central-1"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "EU (Stockholm)"
    value = "eu-north-1"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "EU (Ireland)"
    value = "eu-west-1"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "EU (London)"
    value = "eu-west-2"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "EU (Paris)"
    value = "eu-west-3"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "South America (São Paulo)"
    value = "sa-east-1"
    icon  = "/emojis/1f1e7-1f1f7.png"
  }
  option {
    name  = "US East (N. Virginia)"
    value = "us-east-1"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US East (Ohio)"
    value = "us-east-2"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US West (N. California)"
    value = "us-west-1"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US West (Oregon)"
    value = "us-west-2"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
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
  region = data.coder_parameter.region.value
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

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

resource "coder_agent" "main" {
  count          = data.coder_workspace.me.start_count
  os             = "linux"
  arch           = "amd64"
  auth           = "aws-instance-identity"
  startup_script = <<-EOT
    #!/bin/bash
    set -e
    echo "Agent 'main' started successfully"
    echo "CODER_AGENT_NAME=$CODER_AGENT_NAME"
  EOT

  metadata {
    key          = "agent-identity"
    display_name = "Agent Identity"
    interval     = 60
    timeout      = 5
    script       = "echo main"
  }
}

resource "coder_agent" "dev" {
  count          = data.coder_workspace.me.start_count
  os             = "linux"
  arch           = "amd64"
  auth           = "aws-instance-identity"
  startup_script = <<-EOT
    #!/bin/bash
    set -e
    echo "Agent 'dev' started successfully"
    echo "CODER_AGENT_NAME=$CODER_AGENT_NAME"
  EOT

  metadata {
    key          = "agent-identity"
    display_name = "Agent Identity"
    interval     = 60
    timeout      = 5
    script       = "echo dev"
  }
}

locals {
  aws_availability_zone = "${data.coder_parameter.region.value}a"
  hostname              = lower(data.coder_workspace.me.name)
  linux_user            = "coder"
}

data "cloudinit_config" "user_data" {
  gzip          = false
  base64_encode = false

  boundary = "//"

  part {
    filename     = "userdata.sh"
    content_type = "text/x-shellscript"

    content = templatefile("${path.module}/cloud-init/userdata.sh.tftpl", {
      linux_user       = local.linux_user
      main_init_script = try(coder_agent.main[0].init_script, "")
      dev_init_script  = try(coder_agent.dev[0].init_script, "")
    })
  }
}

resource "aws_vpc" "workspace" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${local.hostname}"
  }
}

resource "aws_subnet" "workspace" {
  vpc_id                  = aws_vpc.workspace.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = local.aws_availability_zone
  map_public_ip_on_launch = true

  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${local.hostname}"
  }
}

resource "aws_internet_gateway" "workspace" {
  vpc_id = aws_vpc.workspace.id

  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${local.hostname}"
  }
}

resource "aws_route_table" "workspace" {
  vpc_id = aws_vpc.workspace.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.workspace.id
  }

  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${local.hostname}"
  }
}

resource "aws_route_table_association" "workspace" {
  subnet_id      = aws_subnet.workspace.id
  route_table_id = aws_route_table.workspace.id
}

resource "aws_security_group" "workspace" {
  name_prefix = "coder-${local.hostname}-"
  description = "Allow SSH access for testing."
  vpc_id      = aws_vpc.workspace.id

  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${local.hostname}"
  }
}

resource "aws_instance" "dev" {
  ami                         = data.aws_ami.ubuntu.id
  availability_zone           = local.aws_availability_zone
  instance_type               = data.coder_parameter.instance_type.value
  subnet_id                   = aws_subnet.workspace.id
  vpc_security_group_ids      = [aws_security_group.workspace.id]
  associate_public_ip_address = true

  user_data = data.cloudinit_config.user_data.rendered
  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${data.coder_workspace.me.name}"
    # Required if you are using our example policy, see template README
    Coder_Provisioned = "true"
  }
  lifecycle {
    ignore_changes = [ami]
  }

  depends_on = [aws_route_table_association.workspace]
}

resource "coder_metadata" "workspace_info" {
  resource_id = aws_instance.dev.id
  item {
    key   = "region"
    value = data.coder_parameter.region.value
  }
  item {
    key   = "instance type"
    value = aws_instance.dev.instance_type
  }
  item {
    key   = "ami"
    value = aws_instance.dev.ami
  }
}

resource "aws_ec2_instance_state" "dev" {
  instance_id = aws_instance.dev.id
  state       = data.coder_workspace.me.transition == "start" ? "running" : "stopped"
}
