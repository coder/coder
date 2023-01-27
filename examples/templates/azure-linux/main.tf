terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.10"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "=3.0.0"
    }
  }
}

variable "location" {
  description = "What location should your workspace live in?"
  default     = "eastus"
  validation {
    condition = contains([
      "eastus",
      "southcentralus",
      "westus2",
      "australiaeast",
      "southeastasia",
      "northeurope",
      "westeurope",
      "centralindia",
      "eastasia",
      "japaneast",
      "brazilsouth",
      "asia",
      "asiapacific",
      "australia",
      "brazil",
      "india",
      "japan",
      "southafrica",
      "switzerland",
      "uae",
    ], var.location)
    error_message = "Invalid location!"
  }
}

variable "instance_type" {
  description = "What instance type should your workspace use?"
  default     = "Standard_B4ms"
  validation {
    condition = contains([
      "Standard_B1ms",
      "Standard_B2ms",
      "Standard_B4ms",
      "Standard_B8ms",
      "Standard_B12ms",
      "Standard_B16ms",
      "Standard_D2as_v5",
      "Standard_D4as_v5",
      "Standard_D8as_v5",
      "Standard_D16as_v5",
      "Standard_D32as_v5",
    ], var.instance_type)
    error_message = "Invalid instance type!"
  }
}

variable "home_size" {
  type        = number
  description = "How large would you like your home volume to be (in GB)?"
  default     = 20
  validation {
    condition     = var.home_size >= 1
    error_message = "Value must be greater than or equal to 1."
  }
}

provider "azurerm" {
  features {}
}

data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  auth = "azure-instance-identity"

  login_before_ready = false
}

locals {
  prefix = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"

  userdata = templatefile("cloud-config.yaml.tftpl", {
    username    = "coder" # Ensure this user/group does not exist in your VM image
    init_script = base64encode(coder_agent.main.init_script)
    hostname    = lower(data.coder_workspace.me.name)
  })
}

resource "azurerm_resource_group" "main" {
  name     = "${local.prefix}-resources"
  location = var.location

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
#
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
    //public_ip_address_id = azurerm_public_ip.main.id
  }

  tags = {
    Coder_Provisioned = "true"
  }
}

resource "azurerm_managed_disk" "home" {
  create_option        = "Empty"
  location             = azurerm_resource_group.main.location
  name                 = "home"
  resource_group_name  = azurerm_resource_group.main.name
  storage_account_type = "StandardSSD_LRS"
  disk_size_gb         = var.home_size
}

// azurerm requires an SSH key (or password) for an admin user or it won't start a VM.  However,
// cloud-init overwrites this anyway, so we'll just use a dummy SSH key.
resource "tls_private_key" "dummy" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "azurerm_linux_virtual_machine" "main" {
  count               = data.coder_workspace.me.transition == "start" ? 1 : 0
  name                = "vm"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  size                = var.instance_type
  // cloud-init overwrites this, so the value here doesn't matter
  admin_username = "adminuser"
  admin_ssh_key {
    public_key = tls_private_key.dummy.public_key_openssh
    username   = "adminuser"
  }

  network_interface_ids = [
    azurerm_network_interface.main.id,
  ]
  computer_name = lower(data.coder_workspace.me.name)
  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }
  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-focal"
    sku       = "20_04-lts-gen2"
    version   = "latest"
  }
  user_data = base64encode(local.userdata)

  tags = {
    Coder_Provisioned = "true"
  }
}

resource "azurerm_virtual_machine_data_disk_attachment" "home" {
  count              = data.coder_workspace.me.transition == "start" ? 1 : 0
  managed_disk_id    = azurerm_managed_disk.home.id
  virtual_machine_id = azurerm_linux_virtual_machine.main[0].id
  lun                = "10"
  caching            = "ReadWrite"
}

resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = azurerm_linux_virtual_machine.main[0].id

  item {
    key   = "type"
    value = azurerm_linux_virtual_machine.main[0].size
  }
}

resource "coder_metadata" "home_info" {
  resource_id = azurerm_managed_disk.home.id

  item {
    key   = "size"
    value = "${var.home_size} GiB"
  }
}
