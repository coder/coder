terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.7.0"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "=3.52.0"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "coder" {
}

data "coder_workspace" "me" {}

data "coder_parameter" "location" {
  description  = "What location should your workspace live in?"
  display_name = "Location"
  name         = "location"
  default      = "eastus"
  mutable      = false
  option {
    value = "eastus"
    name  = "East US"
  }
  option {
    value = "centralus"
    name  = "Central US"
  }
  option {
    value = "southcentralus"
    name  = "South Central US"
  }
  option {
    value = "westus2"
    name  = "West US 2"
  }
}
resource "coder_agent" "main" {
  count          = data.coder_workspace.me.start_count
  arch           = "amd64"
  auth           = "azure-instance-identity"
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
choco feature enable -n=allowGlobalConfirmation
choco install visualstudio2022community --package-parameters "--add=Microsoft.VisualStudio.Workload.ManagedDesktop;includeRecommended --passive --locale en-US"
EOF
}

locals {
  prefix         = "spike"
  admin_username = "coder"
  # Password to log in via RDP
  #
  # Must meet Windows password complexity requirements:
  # https://docs.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/password-must-meet-complexity-requirements#reference
  admin_password = "coderRDP!"
  # User data is used to stop/start AWS instances. See:
  # https://github.com/hashicorp/terraform-provider-aws/issues/22
  user_data_start = <<EOT
# Install Chocolatey package manager before
# the agent starts to use via startup_script
Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
# Reload path so sessions include "choco" and "refreshenv"
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
# Install Git and reload path
choco install -y git
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
${try(coder_agent.main[0].init_script, "")}
EOT
  user_data_end   = <<EOT
shutdown /s
EOT
}
resource "azurerm_resource_group" "main" {
  name     = "${local.prefix}-${data.coder_workspace.me.name}-resources"
  location = data.coder_parameter.location.value
  tags = {
    Coder_Provisioned = "true"
  }
}

// Uncomment here and in the azurerm_network_interface resource to obtain a public IP
resource "azurerm_public_ip" "main" {
  name                = "publicip"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  allocation_method   = "Static"
  tags = {
    Coder_Provisioned = "true"
  }
}
resource "azurerm_virtual_network" "main" {
  name                = "network"
  address_space       = ["10.0.0.0/24"]
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  tags = {
    Coder_Provisioned = "true"
  }
}
resource "azurerm_subnet" "internal" {
  name                 = "internal"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.0.0/29"]
}
resource "azurerm_network_interface" "main" {
  name                = "nic"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.internal.id
    private_ip_address_allocation = "Dynamic"
    // Uncomment for public IP address as well as azurerm_public_ip resource above
    public_ip_address_id = azurerm_public_ip.main.id
  }
  tags = {
    Coder_Provisioned = "true"
  }
}
# Create storage account for boot diagnostics
resource "azurerm_storage_account" "my_storage_account" {
  name                     = "diag${random_id.storage_id.hex}"
  location                 = azurerm_resource_group.main.location
  resource_group_name      = azurerm_resource_group.main.name
  account_tier             = "Standard"
  account_replication_type = "LRS"
}
# Generate random text for a unique storage account name
resource "random_id" "storage_id" {
  keepers = {
    # Generate a new ID only when a new resource group is defined
    resource_group = azurerm_resource_group.main.name
  }
  byte_length = 8
}
# Create virtual machine
resource "azurerm_windows_virtual_machine" "main" {
  name                  = "vm"
  admin_username        = local.admin_username
  admin_password        = local.admin_password
  location              = azurerm_resource_group.main.location
  resource_group_name   = azurerm_resource_group.main.name
  network_interface_ids = [azurerm_network_interface.main.id]
  size                  = "Standard_DS1_v2"
  custom_data           = base64encode(local.user_data_start)
  os_disk {
    name                 = "myOsDisk"
    caching              = "ReadWrite"
    storage_account_type = "Premium_LRS"
  }
  source_image_reference {
    publisher = "MicrosoftWindowsServer"
    offer     = "WindowsServer"
    sku       = "2022-datacenter-azure-edition"
    version   = "latest"
  }
  additional_unattend_content {
    content = "<AutoLogon><Password><Value>${local.admin_password}</Value></Password><Enabled>true</Enabled><LogonCount>1</LogonCount><Username>${local.admin_username}</Username></AutoLogon>"
    setting = "AutoLogon"
  }
  additional_unattend_content {
    content = file("./FirstLogonCommands.xml")
    setting = "FirstLogonCommands"
  }
  boot_diagnostics {
    storage_account_uri = azurerm_storage_account.my_storage_account.primary_blob_endpoint
  }
  tags = {
    Coder_Provisioned = "true"
  }

  lifecycle {
    ignore_changes = [custom_data]
  }

}

# Stop the VM
resource "null_resource" "stop_vm" {
  count      = data.coder_workspace.me.transition == "stop" ? 1 : 0
  depends_on = [azurerm_windows_virtual_machine.main]
  provisioner "local-exec" {
    command = "az vm stop --ids ${azurerm_windows_virtual_machine.main.id}"
  }
}

# Start the VM
resource "null_resource" "start" {
  count      = data.coder_workspace.me.transition == "start" ? 1 : 0
  depends_on = [azurerm_windows_virtual_machine.main]
  provisioner "local-exec" {
    command = "az vm start --ids ${azurerm_windows_virtual_machine.main.id}"
  }
}
