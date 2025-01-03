terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    digitalocean = {
      source = "digitalocean/digitalocean"
    }
  }
}

provider "coder" {}

variable "project_uuid" {
  type        = string
  description = <<-EOF
    DigitalOcean project ID

      $ doctl projects list
  EOF
  sensitive   = true

  validation {
    # make sure length of alphanumeric string is 36 (UUIDv4 size)
    condition     = length(var.project_uuid) == 36
    error_message = "Invalid Digital Ocean Project ID."
  }

}

variable "ssh_key_id" {
  type        = number
  description = <<-EOF
    DigitalOcean SSH key ID (some Droplet images require an SSH key to be set):

    Can be set to "0" for no key.

    Note: Setting this to zero will break Fedora images and notify root passwords via email.

      $ doctl compute ssh-key list
  EOF
  sensitive   = true
  default     = 0

  validation {
    condition     = var.ssh_key_id >= 0
    error_message = "Invalid Digital Ocean SSH key ID, a number is required."
  }
}

data "coder_parameter" "droplet_image" {
  name         = "droplet_image"
  display_name = "Droplet image"
  description  = "Which Droplet image would you like to use?"
  default      = "ubuntu-22-04-x64"
  type         = "string"
  mutable      = false
  option {
    name  = "AlmaLinux 9"
    value = "almalinux-9-x64"
    icon  = "/icon/almalinux.svg"
  }
  option {
    name  = "AlmaLinux 8"
    value = "almalinux-8-x64"
    icon  = "/icon/almalinux.svg"
  }
  option {
    name  = "Fedora 39"
    value = "fedora-39-x64"
    icon  = "/icon/fedora.svg"
  }
  option {
    name  = "Fedora 38"
    value = "fedora-38-x64"
    icon  = "/icon/fedora.svg"
  }
  option {
    name  = "CentOS Stream 9"
    value = "centos-stream-9-x64"
    icon  = "/icon/centos.svg"
  }
  option {
    name  = "CentOS Stream 8"
    value = "centos-stream-8-x64"
    icon  = "/icon/centos.svg"
  }
  option {
    name  = "Debian 12"
    value = "debian-12-x64"
    icon  = "/icon/debian.svg"
  }
  option {
    name  = "Debian 11"
    value = "debian-11-x64"
    icon  = "/icon/debian.svg"
  }
  option {
    name  = "Debian 10"
    value = "debian-10-x64"
    icon  = "/icon/debian.svg"
  }
  option {
    name  = "Rocky Linux 9"
    value = "rockylinux-9-x64"
    icon  = "/icon/rockylinux.svg"
  }
  option {
    name  = "Rocky Linux 8"
    value = "rockylinux-8-x64"
    icon  = "/icon/rockylinux.svg"
  }
  option {
    name  = "Ubuntu 22.04 (LTS)"
    value = "ubuntu-22-04-x64"
    icon  = "/icon/ubuntu.svg"
  }
  option {
    name  = "Ubuntu 20.04 (LTS)"
    value = "ubuntu-20-04-x64"
    icon  = "/icon/ubuntu.svg"
  }
}

data "coder_parameter" "droplet_size" {
  name         = "droplet_size"
  display_name = "Droplet size"
  description  = "Which Droplet configuration would you like to use?"
  default      = "s-1vcpu-1gb"
  type         = "string"
  icon         = "/icon/memory.svg"
  mutable      = false
  # s-1vcpu-512mb-10gb is unsupported in tor1, blr1, lon1, sfo2, and nyc3 regions
  # s-8vcpu-16gb access requires a support ticket with Digital Ocean
  option {
    name  = "1 vCPU, 1 GB RAM"
    value = "s-1vcpu-1gb"
  }
  option {
    name  = "1 vCPU, 2 GB RAM"
    value = "s-1vcpu-2gb"
  }
  option {
    name  = "2 vCPU, 2 GB RAM"
    value = "s-2vcpu-2gb"
  }
  option {
    name  = "2 vCPU, 4 GB RAM"
    value = "s-2vcpu-4gb"
  }
  option {
    name  = "4 vCPU, 8 GB RAM"
    value = "s-4vcpu-8gb"
  }
}

data "coder_parameter" "home_volume_size" {
  name         = "home_volume_size"
  display_name = "Home volume size"
  description  = "How large would you like your home volume to be (in GB)?"
  type         = "number"
  default      = "20"
  mutable      = false
  validation {
    min = 1
    max = 100 # Sizes larger than 100 GB require a support ticket with Digital Ocean
  }
}

data "coder_parameter" "region" {
  name         = "region"
  display_name = "Region"
  description  = "This is the region where your workspace will be created."
  icon         = "/emojis/1f30e.png"
  type         = "string"
  default      = "ams3"
  mutable      = false
  # nyc1, sfo1, and ams2 regions were excluded because they do not support volumes, which are used to persist data while decreasing cost
  option {
    name  = "Canada (Toronto)"
    value = "tor1"
    icon  = "/emojis/1f1e8-1f1e6.png"
  }
  option {
    name  = "Germany (Frankfurt)"
    value = "fra1"
    icon  = "/emojis/1f1e9-1f1ea.png"
  }
  option {
    name  = "India (Bangalore)"
    value = "blr1"
    icon  = "/emojis/1f1ee-1f1f3.png"
  }
  option {
    name  = "Netherlands (Amsterdam)"
    value = "ams3"
    icon  = "/emojis/1f1f3-1f1f1.png"
  }
  option {
    name  = "Singapore"
    value = "sgp1"
    icon  = "/emojis/1f1f8-1f1ec.png"
  }
  option {
    name  = "United Kingdom (London)"
    value = "lon1"
    icon  = "/emojis/1f1ec-1f1e7.png"
  }
  option {
    name  = "United States (California - 2)"
    value = "sfo2"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "United States (California - 3)"
    value = "sfo3"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "United States (New York - 1)"
    value = "nyc1"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "United States (New York - 3)"
    value = "nyc3"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
}

# Configure the DigitalOcean Provider
provider "digitalocean" {
  # Recommended: use environment variable DIGITALOCEAN_TOKEN with your personal access token when starting coderd
  # alternatively, you can pass the token via a variable.
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

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
    key          = "home"
    display_name = "Home Usage"
    interval     = 600 # every 10 minutes
    timeout      = 30  # df can take a while on large filesystems
    script       = "coder stat disk --path /home/${lower(data.coder_workspace_owner.me.name)}"
  }
}

# See https://registry.coder.com/modules/code-server
module "code-server" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/code-server/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id = coder_agent.main.id
  order    = 1
}

# See https://registry.coder.com/modules/jetbrains-gateway
module "jetbrains_gateway" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/jetbrains-gateway/coder"

  # JetBrains IDEs to make available for the user to select
  jetbrains_ides = ["IU", "PY", "WS", "PS", "RD", "CL", "GO", "RM"]
  default        = "IU"

  # Default folder to open when starting a JetBrains IDE
  folder = "/home/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id   = coder_agent.main.id
  agent_name = "main"
  order      = 2
}

resource "digitalocean_volume" "home_volume" {
  region                   = data.coder_parameter.region.value
  name                     = "coder-${data.coder_workspace.me.id}-home"
  size                     = data.coder_parameter.home_volume_size.value
  initial_filesystem_type  = "ext4"
  initial_filesystem_label = "coder-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
}

resource "digitalocean_droplet" "workspace" {
  region = data.coder_parameter.region.value
  count  = data.coder_workspace.me.start_count
  name   = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}"
  image  = data.coder_parameter.droplet_image.value
  size   = data.coder_parameter.droplet_size.value

  volume_ids = [digitalocean_volume.home_volume.id]
  user_data = templatefile("cloud-config.yaml.tftpl", {
    username          = lower(data.coder_workspace_owner.me.name)
    home_volume_label = digitalocean_volume.home_volume.initial_filesystem_label
    init_script       = base64encode(coder_agent.main.init_script)
    coder_agent_token = coder_agent.main.token
  })
  # Required to provision Fedora.
  ssh_keys = var.ssh_key_id > 0 ? [var.ssh_key_id] : []
}

resource "digitalocean_project_resources" "project" {
  project = var.project_uuid
  # Workaround for terraform plan when using count.
  resources = length(digitalocean_droplet.workspace) > 0 ? [
    digitalocean_volume.home_volume.urn,
    digitalocean_droplet.workspace[0].urn
    ] : [
    digitalocean_volume.home_volume.urn
  ]
}

resource "coder_metadata" "workspace-info" {
  count       = data.coder_workspace.me.start_count
  resource_id = digitalocean_droplet.workspace[0].id

  item {
    key   = "region"
    value = digitalocean_droplet.workspace[0].region
  }
  item {
    key   = "image"
    value = digitalocean_droplet.workspace[0].image
  }
}

resource "coder_metadata" "volume-info" {
  resource_id = digitalocean_volume.home_volume.id

  item {
    key   = "size"
    value = "${digitalocean_volume.home_volume.size} GiB"
  }
}
