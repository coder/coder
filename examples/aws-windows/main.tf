terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

variable "access_key" {
  description = <<EOT
Create an AWS access key to provision resources with Coder:
- https://console.aws.amazon.com/iam/home#/users

AWS Access Key
EOT
  sensitive   = true
}

variable "secret_key" {
  description = <<EOT
AWS Secret Key
EOT
  sensitive   = true
}

variable "region" {
  description = "What region should your workspace live in?"
  default     = "us-east-1"
  validation {
    condition     = contains(["us-east-1", "us-east-2", "us-west-1", "us-west-2"], var.region)
    error_message = "Invalid region!"
  }
}

provider "aws" {
  region     = var.region
  access_key = var.access_key
  secret_key = var.secret_key
}

data "coder_workspace" "me" {
}

data "coder_agent_script" "dev" {
  arch = "amd64"
  auth = "aws-instance-identity"
  os   = "windows"
}

# assign a random name for the workspace
resource "random_string" "random" {
  length  = 8
  special = false
}

data "aws_ami" "windows" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["Windows_Server-2019-English-Full-Base-*"]
  }
}

resource "coder_agent" "dev" {
  count       = data.coder_workspace.me.transition == "start" ? 1 : 0
  instance_id = aws_instance.dev[0].id
}

locals {
  user_data_start = <<EOT
<powershell>
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
${data.coder_agent_script.dev.value}
</powershell>
<persist>true</persist>
EOT

  user_data_end = <<EOT
<powershell>
shutdown /s
</powershell>
<persist>true</persist>
EOT
}

resource "aws_instance" "dev" {
  # count             = data.coder_workspace.me.transition == "start" ? 1 : 0
  ami               = data.aws_ami.windows.id
  availability_zone = "${var.region}a"
  instance_type     = "t3.micro"
  count             = 1

  user_data = data.coder_workspace.me.transition == "start" ? local.user_data_start : local.user_data_end
  tags = {
    Name = "coder-${lower(random_string.random.result)}"
  }

}
