terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.9"
    }
  }
}

# Last updated 2022-05-31
# aws ec2 describe-regions | jq -r '[.Regions[].RegionName] | sort'
variable "region" {
  description = "What region should your workspace live in?"
  default     = "us-east-1"
  validation {
    condition = contains([
      "ap-northeast-1",
      "ap-northeast-2",
      "ap-northeast-3",
      "ap-south-1",
      "ap-southeast-1",
      "ap-southeast-2",
      "ca-central-1",
      "eu-central-1",
      "eu-north-1",
      "eu-west-1",
      "eu-west-2",
      "eu-west-3",
      "sa-east-1",
      "us-east-1",
      "us-east-2",
      "us-west-1",
      "us-west-2"
    ], var.region)
    error_message = "Invalid region!"
  }
}

variable "instance_type" {
  description = "What instance type should your workspace use?"
  default     = "t3.micro"
  validation {
    condition = contains([
      "t3.micro",
      "t3.small",
      "t3.medium",
      "t3.large",
      "t3.xlarge",
      "t3.2xlarge",
    ], var.instance_type)
    error_message = "Invalid instance type!"
  }
}

provider "aws" {
  region = var.region
}

data "coder_workspace" "me" {
}

data "aws_ami" "windows" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["Windows_Server-2019-English-Full-Base-*"]
  }
}

resource "coder_agent" "main" {
  arch           = "amd64"
  auth           = "aws-instance-identity"
  os             = "windows"
  startup_script = <<EOF
# Set admin password
Get-LocalUser -Name "Administrator" | Set-LocalUser -Password (ConvertTo-SecureString -AsPlainText "${local.admin_password}" -Force)
# To disable password entirely, see https://serverfault.com/a/968240

# Enable RDP
Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Terminal Server' -name "fDenyTSConnections" -value 0

# Enable RDP through Windows Firewall
Enable-NetFirewallRule -DisplayGroup "Remote Desktop"

# Disable Network Level Authentication (NLA)
# Clients will connect via Coder's tunnel
(Get-WmiObject -class "Win32_TSGeneralSetting" -Namespace root\cimv2\terminalservices -ComputerName $env:COMPUTERNAME -Filter "TerminalName='RDP-tcp'").SetUserAuthenticationRequired(0)

# Install Chocolatey package manager
Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
EOF
}

locals {
  # Password to log in via RDP
  #
  # Must meet Windows password complexity requirements:
  # https://docs.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/password-must-meet-complexity-requirements#reference
  admin_password = "coderRDP!"

  # User data is used to stop/start AWS instances. See:
  # https://github.com/hashicorp/terraform-provider-aws/issues/22
  user_data_start = <<EOT
<powershell>
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
${coder_agent.main.init_script}
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
  ami               = data.aws_ami.windows.id
  availability_zone = "${var.region}a"
  instance_type     = var.instance_type

  user_data = data.coder_workspace.me.transition == "start" ? local.user_data_start : local.user_data_end
  tags = {
    Name = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
    # Required if you are using our example policy, see template README
    Coder_Provisioned = "true"
  }

}

resource "coder_metadata" "workspace_info" {
  resource_id = aws_instance.dev.id
  item {
    key       = "Administrator password"
    value     = local.admin_password
    sensitive = true
  }
  item {
    key   = "Region"
    value = var.region
  }
  item {
    key   = "Instance type"
    value = aws_instance.dev.instance_type
  }
  item {
    key   = "Disk"
    value = "${aws_instance.dev.root_block_device[0].volume_size} GiB"
  }
}
