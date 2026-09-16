terraform {
  required_version = ">= 1.3.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.16.0"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "4.72.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "4.2.1"
    }
  }
}

provider "azurerm" {
  features {}
  # Credentials are supplied by the external Terraform provisioner via ARM_*.
  subscription_id = var.azure_subscription_id
}
provider "coder" {}

data "coder_workspace" "me" {}
data "coder_workspace_tags" "tags" {
  tags = var.provisioner_tags
}

locals {
  location = var.azure_location
  prefix   = "coder-sandbox-${data.coder_workspace.me.id}"
  tags = {
    Coder_Provisioned  = "true"
    Coder_Workspace_ID = data.coder_workspace.me.id
  }
  bootstrap_script = templatefile("${path.module}/launch-bootstrap.tftpl", {
    bootstrap_gzip_base64 = base64gzip(file("${path.module}/bootstrap.sh"))
    bootstrap_sha256      = filesha256("${path.module}/bootstrap.sh")
    source_config_base64 = base64encode(jsonencode({
      repository = var.coder_source_repository
      commit     = var.coder_source_commit
    }))
  })
  cloud_config = data.coder_workspace.me.start_count > 0 ? templatefile("${path.module}/cloud-config.yaml.tftpl", {
    hostname           = lower(data.coder_workspace.me.name)
    agent_init_base64  = base64encode(coder_agent.dev[0].init_script)
    agent_env_base64   = base64encode("CODER_AGENT_TOKEN=${coder_agent.dev[0].token}\n")
    state_setup_base64 = filebase64("${path.module}/prepare-state.sh")
  }) : ""
}

resource "coder_agent" "dev" {
  count = data.coder_workspace.me.start_count
  arch  = "amd64"
  auth  = "token"
  os    = "linux"

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
    key          = "disk"
    display_name = "Disk Usage"
    interval     = 60
    timeout      = 10
    script       = "coder stat disk --path /var/lib"
  }
  metadata {
    key          = "sandbox-bootstrap"
    display_name = "Sandbox Host Setup"
    interval     = 10
    timeout      = 5
    script       = "sudo cat /var/lib/coder-sandbox-host/status 2>/dev/null || printf 'pending: waiting for bootstrap launch\\n'"
  }
}

resource "coder_script" "launch_bootstrap" {
  count              = data.coder_workspace.me.start_count
  agent_id           = coder_agent.dev[0].id
  display_name       = "Set up sandbox host"
  run_on_start       = true
  start_blocks_login = false
  timeout            = 120
  log_path           = "/home/coder/sandbox-bootstrap-launch.log"
  script             = local.bootstrap_script

  lifecycle {
    precondition {
      condition     = length(local.bootstrap_script) < 131072
      error_message = "Bootstrap launch script exceeds Linux's per-argument limit. Split its compressed payload before publishing."
    }
  }
}

resource "azurerm_resource_group" "host" {
  name     = "${local.prefix}-resources"
  location = local.location
  tags     = local.tags
}

resource "azurerm_virtual_network" "host" {
  name                = "sandbox-host-network"
  location            = local.location
  resource_group_name = azurerm_resource_group.host.name
  address_space       = ["10.0.0.0/24"]
  tags                = local.tags
}

resource "azurerm_subnet" "host" {
  name                            = "sandbox-host-subnet"
  resource_group_name             = azurerm_resource_group.host.name
  virtual_network_name            = azurerm_virtual_network.host.name
  address_prefixes                = ["10.0.0.0/29"]
  default_outbound_access_enabled = false
}

# The public IP is an explicit outbound path. No inbound ports are allowed.
resource "azurerm_public_ip" "outbound" {
  name                = "sandbox-host-outbound"
  location            = local.location
  resource_group_name = azurerm_resource_group.host.name
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = local.tags
}

resource "azurerm_network_security_group" "host" {
  name                = "sandbox-host-deny-inbound"
  location            = local.location
  resource_group_name = azurerm_resource_group.host.name
  tags                = local.tags

  security_rule {
    name                       = "DenyAllInbound"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}

resource "azurerm_network_interface" "host" {
  name                = "sandbox-host-nic"
  location            = local.location
  resource_group_name = azurerm_resource_group.host.name
  tags                = local.tags
  ip_configuration {
    name      = "primary"
    subnet_id = azurerm_subnet.host.id
    # Stable across compute replacements so the inner Coder access URL stays valid.
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.0.0.4"
    public_ip_address_id          = azurerm_public_ip.outbound.id
  }
}

resource "azurerm_network_interface_security_group_association" "host" {
  network_interface_id      = azurerm_network_interface.host.id
  network_security_group_id = azurerm_network_security_group.host.id
}

resource "azurerm_managed_disk" "state" {
  name                 = "sandbox-host-state"
  location             = local.location
  resource_group_name  = azurerm_resource_group.host.name
  storage_account_type = "Premium_LRS"
  create_option        = "Empty"
  disk_size_gb         = 128
  tags                 = local.tags
}

# Azure requires an admin SSH public key. Coder access uses its outbound agent;
# this generated private key remains in Terraform state and is never output.
resource "tls_private_key" "unused_admin" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Inline attachment keeps the state disk with the VM until compute deletion.
# The managed disk itself remains a separate persistent Terraform resource.
resource "azurerm_virtual_machine" "host" {
  count                            = data.coder_workspace.me.start_count
  name                             = "sandbox-host"
  location                         = local.location
  resource_group_name              = azurerm_resource_group.host.name
  vm_size                          = var.azure_vm_size
  network_interface_ids            = [azurerm_network_interface.host.id]
  tags                             = local.tags
  delete_os_disk_on_termination    = true
  delete_data_disks_on_termination = false

  os_profile {
    computer_name  = lower(data.coder_workspace.me.name)
    admin_username = "coder"
    # This resource base64-encodes custom_data internally.
    custom_data = local.cloud_config
  }
  os_profile_linux_config {
    disable_password_authentication = true
    ssh_keys {
      path     = "/home/coder/.ssh/authorized_keys"
      key_data = tls_private_key.unused_admin.public_key_openssh
    }
  }
  storage_os_disk {
    name              = "sandbox-host-os"
    create_option     = "FromImage"
    caching           = "ReadWrite"
    managed_disk_type = "Premium_LRS"
    disk_size_gb      = 128
  }
  storage_data_disk {
    name            = azurerm_managed_disk.state.name
    managed_disk_id = azurerm_managed_disk.state.id
    create_option   = "Attach"
    lun             = 10
    caching         = "None"
    disk_size_gb    = 128
  }
  storage_image_reference {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }

  # Establish the deny-all rule before compute can start listening.
  depends_on = [azurerm_network_interface_security_group_association.host]

  lifecycle {
    precondition {
      condition     = length(base64encode(local.cloud_config)) <= 87380
      error_message = "Azure custom data exceeds 64 KiB; keep large assets in coder_script delivery."
    }
  }
}

resource "coder_metadata" "host" {
  count       = data.coder_workspace.me.start_count
  resource_id = azurerm_virtual_machine.host[0].id
  item {
    key   = "host"
    value = "Ubuntu 24.04 / ${var.azure_vm_size} / ${var.azure_location}"
  }
  item {
    key   = "disk"
    value = "128 GiB OS + 128 GiB persistent sandbox state"
  }
  item {
    key   = "network"
    value = "Outbound only; all inbound ports denied"
  }
}
