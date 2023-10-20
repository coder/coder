terraform {
  required_providers {
    fly = {
      source = "fly-apps/fly"
    }
    coder = {
      source = "coder/coder"
    }
  }
}

provider "fly" {
  fly_api_token = var.fly_api_token == "" ? null : var.fly_api_token
}

provider "coder" {
}

resource "fly_app" "workspace" {
  name = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}"
  org  = var.fly_org
}

resource "fly_volume" "home-volume" {
  app    = fly_app.workspace.name
  name   = "coder_${lower(data.coder_workspace.me.owner)}_${lower(replace(data.coder_workspace.me.name, "-", "_"))}_home"
  size   = data.coder_parameter.volume-size.value
  region = data.coder_parameter.region.value
}

resource "fly_machine" "workspace" {
  count    = data.coder_workspace.me.start_count
  app      = fly_app.workspace.name
  region   = data.coder_parameter.region.value
  name     = data.coder_workspace.me.name
  image    = data.coder_parameter.docker-image.value
  cpus     = data.coder_parameter.cpu.value
  cputype  = data.coder_parameter.cputype.value
  memorymb = data.coder_parameter.memory.value * 1024
  env = {
    CODER_AGENT_TOKEN = "${coder_agent.main.token}"
  }
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  services = [
    {
      ports = [
        {
          port     = 443
          handlers = ["tls", "http"]
        },
        {
          port     = 80
          handlers = ["http"]
        }

      ]
      protocol        = "tcp",
      "internal_port" = 80
    },
    {
      ports = [
        {
          port     = 8080
          handlers = ["tls", "http"]
        }
      ]
      protocol        = "tcp",
      "internal_port" = 8080
    }
  ]
  mounts = [
    {
      volume = fly_volume.home-volume.id
      path   = "/home/coder"
    }
  ]
}

variable "fly_api_token" {
  type        = string
  description = <<-EOF
The Fly.io API token to use for deploying the workspace. You can generate one by running:

$ flyctl auth token
EOF
  sensitive   = true
}

variable "fly_org" {
  type        = string
  description = <<-EOF
The Fly.io organization slug to deploy the workspace in. List organizations by running:

$ flyctl orgs list
EOF
}

data "coder_parameter" "docker-image" {
  name         = "docker-image"
  display_name = "Docker image"
  description  = "The docker image to use for the workspace"
  default      = "codercom/code-server:latest"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/docker.svg"
}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU"
  description  = "The number of CPUs to allocate to the workspace (1-8)"
  type         = "number"
  default      = "1"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/cpu-3.svg"
  mutable      = true
  validation {
    min = 1
    max = 8
  }
}

data "coder_parameter" "cputype" {
  name         = "cputype"
  display_name = "CPU type"
  description  = "Which CPU type do you want?"
  default      = "shared"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/cpu-1.svg"
  mutable      = true
  option {
    name  = "Shared"
    value = "shared"
  }
  option {
    name  = "Performance"
    value = "performance"
  }
}

data "coder_parameter" "memory" {
  name         = "memory"
  display_name = "Memory"
  description  = "The amount of memory to allocate to the workspace in GB (up to 16GB)"
  type         = "number"
  default      = "2"
  icon         = "/icon/memory.svg"
  mutable      = true
  validation {
    min = data.coder_parameter.cputype.value == "performance" ? 2 : 1 # if the CPU type is performance, the minimum memory is 2GB
    max = 16
  }
}

data "coder_parameter" "volume-size" {
  name         = "volume-size"
  display_name = "Home volume size"
  description  = "The size of the volume to create for the workspace in GB (1-20)"
  type         = "number"
  default      = "1"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/database.svg"
  validation {
    min = 1
    max = 20
  }
}

# You can see all available regions here: https://fly.io/docs/reference/regions/
data "coder_parameter" "region" {
  name         = "region"
  display_name = "Region"
  description  = "The region to deploy the workspace in"
  default      = "ams"
  icon         = "/emojis/1f30e.png"
  option {
    name  = "Amsterdam, Netherlands"
    value = "ams"
    icon  = "/emojis/1f1f3-1f1f1.png"
  }
  option {
    name  = "Frankfurt, Germany"
    value = "fra"
    icon  = "/emojis/1f1e9-1f1ea.png"
  }
  option {
    name  = "Paris, France"
    value = "cdg"
    icon  = "/emojis/1f1eb-1f1f7.png"
  }
  option {
    name  = "Denver, Colorado (US)"
    value = "den"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Dallas, Texas (US)"
    value = "dfw"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Hong Kong"
    value = "hkg"
    icon  = "/emojis/1f1ed-1f1f0.png"
  }
  option {
    name  = "Los Angeles, California (US)"
    value = "lax"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "London, United Kingdom"
    value = "lhr"
    icon  = "/emojis/1f1ec-1f1e7.png"
  }
  option {
    name  = "Chennai, India"
    value = "maa"
    icon  = "/emojis/1f1ee-1f1f3.png"
  }
  option {
    name  = "Tokyo, Japan"
    value = "nrt"
    icon  = "/emojis/1f1ef-1f1f5.png"
  }
  option {
    name  = "Chicago, Illinois (US)"
    value = "ord"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Seattle, Washington (US)"
    value = "sea"
    icon  = "/emojis/1f1fa-1f1f8.png"
  }
  option {
    name  = "Singapore"
    value = "sin"
    icon  = "/emojis/1f1f8-1f1ec.png"
  }
  option {
    name  = "Sydney, Australia"
    value = "syd"
    icon  = "/emojis/1f1e6-1f1fa.png"
  }
  option {
    name  = "Toronto, Canada"
    value = "yyz"
    icon  = "/emojis/1f1e8-1f1e6.png"
  }
}

resource "coder_app" "code-server" {
  count        = 1
  agent_id     = coder_agent.main.id
  display_name = "code-server"
  slug         = "code-server"
  url          = "http://localhost:8080?folder=/home/coder/"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:8080/healthz"
    interval  = 3
    threshold = 10
  }
}

resource "coder_agent" "main" {
  arch                   = data.coder_provisioner.me.arch
  os                     = "linux"
  startup_script_timeout = 180
  startup_script         = <<-EOT
    set -e
    # Start code-server
    code-server --auth none >/tmp/code-server.log 2>&1 &
    # Set the hostname to the workspace name
    sudo hostname -b "${data.coder_workspace.me.name}-fly"
    echo "127.0.0.1  ${data.coder_workspace.me.name}-fly" | sudo tee -a /etc/hosts
    # Install the Fly CLI and add it to the PATH
    curl -L https://fly.io/install.sh | sh
    echo "export PATH=$PATH:/home/coder/.fly/bin" >> /home/coder/.bashrc
    source /home/coder/.bashrc
  EOT

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
      df | awk '$NF=="/home/coder" {printf "%s", $5}'
    EOT
  }
}

resource "coder_metadata" "workspace" {
  count       = data.coder_workspace.me.start_count
  resource_id = fly_app.workspace.id
  icon        = data.coder_parameter.region.option[index(data.coder_parameter.region.option.*.value, data.coder_parameter.region.value)].icon
  item {
    key   = "Region"
    value = data.coder_parameter.region.option[index(data.coder_parameter.region.option.*.value, data.coder_parameter.region.value)].name
  }
  item {
    key   = "CPU Type"
    value = data.coder_parameter.cputype.option[index(data.coder_parameter.cputype.option.*.value, data.coder_parameter.cputype.value)].name
  }
  item {
    key   = "CPU Count"
    value = data.coder_parameter.cpu.value
  }
  item {
    key   = "Memory (GB)"
    value = data.coder_parameter.memory.value
  }
  item {
    key   = "Volume Size (GB)"
    value = data.coder_parameter.volume-size.value
  }
}

data "coder_provisioner" "me" {
}

data "coder_workspace" "me" {
}
