terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    azurerm = {
      source = "hashicorp/azurerm"
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

data "coder_parameter" "data_disk_size" {
  description  = "Size of your data (F:) drive in GB"
  display_name = "Data disk size"
  name         = "data_disk_size"
  default      = 20
  mutable      = "false"
  type         = "number"
  validation {
    min = 5
    max = 5000
  }
}

resource "coder_agent" "main" {
  arch = "amd64"
  auth = "azure-instance-identity"
  os   = "windows"
}

resource "random_password" "admin_password" {
  length  = 16
  special = true
  # https://docs.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/password-must-meet-complexity-requirements#reference
  # we remove characters that require special handling in XML, as this is how we pass it to the VM
  # namely: <>&'"
  override_special = "~!@#$%^*_-+=`|\\(){}[]:;,.?/"
}

locals {
  prefix         = "coder-win"
  admin_username = "coder"
}

resource "azurerm_resource_group" "main" {
  name     = "${local.prefix}-${data.coder_workspace.me.id}"
  location = data.coder_parameter.location.value
  tags = {
    Coder_Provisioned = "true"
  }
}

// Uncomment here and in the azurerm_network_interface resource to obtain a public IP
#resource "azurerm_public_ip" "main" {
#  name                = "publicip"
#  resource_group_name = azurerm_resource_group.main.name
#  location            = azurerm_resource_group.main.location
#  allocation_method   = "Static"
#  tags = {
#    Coder_Provisioned = "true"
#  }
#}
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
    #    public_ip_address_id = azurerm_public_ip.main.id
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

resource "azurerm_managed_disk" "data" {
  name                 = "data_disk"
  location             = azurerm_resource_group.main.location
  resource_group_name  = azurerm_resource_group.main.name
  storage_account_type = "Standard_LRS"
  create_option        = "Empty"
  disk_size_gb         = data.coder_parameter.data_disk_size.value
}

# Create virtual machine
resource "azurerm_windows_virtual_machine" "main" {
  name                  = "vm"
  admin_username        = local.admin_username
  admin_password        = random_password.admin_password.result
  location              = azurerm_resource_group.main.location
  resource_group_name   = azurerm_resource_group.main.name
  network_interface_ids = [azurerm_network_interface.main.id]
  size                  = "Standard_DS1_v2"
  custom_data = base64encode(
    templatefile("${path.module}/Initialize.ps1.tftpl", { init_script = coder_agent.main.init_script })
  )
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
    content = "<AutoLogon><Password><Value>${random_password.admin_password.result}</Value></Password><Enabled>true</Enabled><LogonCount>1</LogonCount><Username>${local.admin_username}</Username></AutoLogon>"
    setting = "AutoLogon"
  }
  additional_unattend_content {
    content = file("${path.module}/FirstLogonCommands.xml")
    setting = "FirstLogonCommands"
  }
  boot_diagnostics {
    storage_account_uri = azurerm_storage_account.my_storage_account.primary_blob_endpoint
  }
  tags = {
    Coder_Provisioned = "true"
  }
}

resource "coder_metadata" "rdp_login" {
  resource_id = azurerm_windows_virtual_machine.main.id
  item {
    key   = "Username"
    value = local.admin_username
  }
  item {
    key       = "Password"
    value     = random_password.admin_password.result
    sensitive = true
  }
}

resource "azurerm_virtual_machine_data_disk_attachment" "main_data" {
  managed_disk_id    = azurerm_managed_disk.data.id
  virtual_machine_id = azurerm_windows_virtual_machine.main.id
  lun                = "10"
  caching            = "ReadWrite"
}

# Stop the VM
resource "null_resource" "stop_vm" {
  count      = data.coder_workspace.me.transition == "stop" ? 1 : 0
  depends_on = [azurerm_windows_virtual_machine.main]
  provisioner "local-exec" {
    # Use deallocate so the VM is not charged
    command = "az vm deallocate --ids ${azurerm_windows_virtual_machine.main.id}"
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
