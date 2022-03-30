terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

variable "access_key" {
  description = <<EOT
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

resource "aws_ec2_host" "dedicated" {
  instance_type     = "mac1.metal"
  availability_zone = "us-west-2a"
  host_recovery     = "off"
  auto_placement    = "on"
}

provider "aws" {
  region     = "us-west-2"
  access_key = var.access_key
  secret_key = var.secret_key
}

data "coder_workspace" "me" {
}

data "coder_agent_script" "dev" {
  arch = "amd64"
  auth = "aws-instance-identity"
  os   = "darwin"
}

# assign a random name for the workspace
resource "random_string" "random" {
  length  = 8
  special = false
}

data "aws_ami" "mac" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name = "name"
    values = [
      "amzn-ec2-macos-12*"
    ]
  }
  filter {
    name = "owner-alias"
    values = [
      "amazon",
    ]
  }
}


resource "coder_agent" "dev" {
  count       = data.coder_workspace.me.transition == "start" ? 1 : 0
  instance_id = aws_instance.dev[0].id
}

locals {

  user_data_start = <<EOT
${data.coder_agent_script.dev.value}
EOT

  user_data_end = <<EOT
sudo shutdown -h now
EOT
}

resource "aws_instance" "dev" {
  ami               = data.aws_ami.mac.id
  availability_zone = "us-west-2a"
  host_id           = aws_ec2_host.dedicated.id
  instance_type     = "mac1.metal"
  count             = 1

  # for debugging
  # TODO: remove
  # key_name = "bens-macbook"


  # user data is not valid now.
  # part of it is because /usr/bin/env/sh is not a valid path
  # on MacOS, among other things
  # user_data = data.coder_workspace.me.transition == "start" ? local.user_data_start : local.user_data_end
  tags = {
    Name = "coder-${lower(random_string.random.result)}"
  }

}
