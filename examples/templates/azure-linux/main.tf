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

data "coder_parameter" "location" {
  name         = "location"
  display_name = "Location"
  description  = "What location should your workspace live in?"
  default      = "eastus"
  icon         = "/emojis/1f310.png"
  mutable      = false
  option {
    name  = "US (Virginia)"
    value = "eastus"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US (Virginia) 2"
    value = "eastus2"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US (Texas)"
    value = "southcentralus"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US (Washington)"
    value = "westus2"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US (Arizona)"
    value = "westus3"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "US (Iowa)"
    value = "centralus"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Canada (Toronto)"
    value = "canadacentral"
    icon  = "/emojis/1f1e8-1f1e6.png"
  }
  option {
    name  = "Brazil (Sao Paulo)"
    value = "brazilsouth"
    icon  = "/emojis/1f1e7-1f1f7.png"
  }
  option {
    name  = "East Asia (Hong Kong)"
    value = "eastasia"
    icon  = "/emojis/1f1f0-1f1f7.png"
  }
  option {
    name  = "Southeast Asia (Singapore)"
    value = "southeastasia"
    icon  = "/emojis/1f1f0-1f1f7.png"
  }
  option {
    name  = "Australia (New South Wales)"
    value = "australiaeast"
    icon  = "/emojis/1f1e6-1f1fa.png"
  }
  option {
    name  = "China (Hebei)"
    value = "chinanorth3"
    icon  = "/emojis/1f1e8-1f1f3.png"
  }
  option {
    name  = "India (Pune)"
    value = "centralindia"
    icon  = "/emojis/1f1ee-1f1f3.png"
  }
  option {
    name  = "Japan (Tokyo)"
    value = "japaneast"
    icon  = "/emojis/1f1ef-1f1f5.png"
  }
  option {
    name  = "Korea (Seoul)"
    value = "koreacentral"
    icon  = "/emojis/1f1f0-1f1f7.png"
  }
  option {
    name  = "Europe (Ireland)"
    value = "northeurope"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "Europe (Netherlands)"
    value = "westeurope"
    icon  = "/emojis/1f1ea-1f1fa.png"
  }
  option {
    name  = "France (Paris)"
    value = "francecentral"
    icon  = "/emojis/1f1eb-1f1f7.png"
  }
  option {
    name  = "Germany (Frankfurt)"
    value = "germanywestcentral"
    icon  = "/emojis/1f1e9-1f1ea.png"
  }
  option {
    name  = "Norway (Oslo)"
    value = "norwayeast"
    icon  = "/emojis/1f1f3-1f1f4.png"
  }
  option {
    name  = "Sweden (Gävle)"
    value = "swedencentral"
    icon  = "/emojis/1f1f8-1f1ea.png"
  }
  option {
    name  = "Switzerland (Zurich)"
    value = "switzerlandnorth"
    icon  = "/emojis/1f1e8-1f1ed.png"
  }
  option {
    name  = "Qatar (Doha)"
    value = "qatarcentral"
    icon  = "/emojis/1f1f6-1f1e6.png"
  }
  option {
    name  = "UAE (Dubai)"
    value = "uaenorth"
    icon  = "/emojis/1f1e6-1f1ea.png"
  }
  option {
    name  = "South Africa (Johannesburg)"
    value = "southafricanorth"
    icon  = "/emojis/1f1ff-1f1e6.png"
  }
  option {
    name  = "UK (London)"
    value = "uksouth"
    icon  = "/emojis/1f1ec-1f1e7.png"
  }
}

data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance type"
  description  = "What instance type should your workspace use?"
  default      = "Standard_B4ms"
  icon         = "/icon/azure.png"
  mutable      = false
  option {
    name  = "Standard_B1ms (1 vCPU, 2 GiB RAM)"
    value = "Standard_B1ms"
  }
  option {
    name  = "Standard_B2ms (2 vCPU, 8 GiB RAM)"
    value = "Standard_B2ms"
  }
  option {
    name  = "Standard_B4ms (4 vCPU, 16 GiB RAM)"
    value = "Standard_B4ms"
  }
  option {
    name  = "Standard_B8ms (8 vCPU, 32 GiB RAM)"
    value = "Standard_B8ms"
  }
  option {
    name  = "Standard_B12ms (12 vCPU, 48 GiB RAM)"
    value = "Standard_B12ms"
  }
  option {
    name  = "Standard_B16ms (16 vCPU, 64 GiB RAM)"
    value = "Standard_B16ms"
  }
  option {
    name  = "Standard_D2as_v5 (2 vCPU, 8 GiB RAM)"
    value = "Standard_D2as_v5"
  }
  option {
    name  = "Standard_D4as_v5 (4 vCPU, 16 GiB RAM)"
    value = "Standard_D4as_v5"
  }
  option {
    name  = "Standard_D8as_v5 (8 vCPU, 32 GiB RAM)"
    value = "Standard_D8as_v5"
  }
  option {
    name  = "Standard_D16as_v5 (16 vCPU, 64 GiB RAM)"
    value = "Standard_D16as_v5"
  }
  option {
    name  = "Standard_D32as_v5 (32 vCPU, 128 GiB RAM)"
    value = "Standard_D32as_v5"
  }
}

data "coder_parameter" "home_size" {
  name         = "home_size"
  display_name = "Home volume size"
  description  = "How large would you like your home volume to be (in GB)?"
  default      = 20
  type         = "number"
  icon         = "/icon/azure.png"
  mutable      = false
  validation {
    min = 1
    max = 1024
  }
}

provider "azurerm" {
  features {}
}

data "coder_workspace" "me" {
}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  auth = "azure-instance-identity"

  metadata {
    key          = "cpu"
    display_name = "CPU Usage"
    interval     = 5
    timeout      = 5
    script       = <<-EOT
      #!/bin/bash
      set -e
      top -bn1 | grep "Cpu(s)" | awk '{print $2 + $4 "%"}'
    EOT
  }
  metadata {
    key          = "memory"
    display_name = "Memory Usage"
    interval     = 5
    timeout      = 5
    script       = <<-EOT
      #!/bin/bash
      set -e
      free -m | awk 'NR==2{printf "%.2f%%\t", $3*100/$2 }'
    EOT
  }
  metadata {
    key          = "disk"
    display_name = "Disk Usage"
    interval     = 600 # every 10 minutes
    timeout      = 30  # df can take a while on large filesystems
    script       = <<-EOT
      #!/bin/bash
      set -e
      df /home/coder | awk '$NF=="/"{printf "%s", $5}'
    EOT
  }
}

locals {
  prefix = "coder-${data.coder_workspace_owner.me.name}-${data.coder_workspace.me.name}"

  userdata = templatefile("cloud-config.yaml.tftpl", {
    username    = "coder" # Ensure this user/group does not exist in your VM image
    init_script = base64encode(coder_agent.main.init_script)
    hostname    = lower(data.coder_workspace.me.name)
  })
}

resource "azurerm_resource_group" "main" {
  name     = "${local.prefix}-resources"
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
  disk_size_gb         = data.coder_parameter.home_size.value
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
  size                = data.coder_parameter.instance_type.value
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
    value = "${data.coder_parameter.home_size.value} GiB"
  }
}
