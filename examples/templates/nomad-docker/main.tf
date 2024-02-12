terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    nomad = {
      source = "hashicorp/nomad"
    }
  }
}

variable "nomad_provider_address" {
  type        = string
  description = "Nomad provider address. e.g., http://IP:PORT"
  default     = "http://localhost:4646"
}

variable "nomad_provider_http_auth" {
  type        = string
  description = "Nomad provider http_auth in the form of `user:password`"
  sensitive   = true
  default     = ""
}

provider "coder" {}

provider "nomad" {
  address   = var.nomad_provider_address
  http_auth = var.nomad_provider_http_auth == "" ? null : var.nomad_provider_http_auth

  # Fix reading the NOMAD_NAMESPACE and the NOMAD_REGION env var from the coder's allocation.
  ignore_env_vars = {
    "NOMAD_NAMESPACE" = true
    "NOMAD_REGION"    = true
  }
}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU"
  description  = "The number of CPU cores"
  default      = "1"
  icon         = "/icon/memory.svg"
  mutable      = true
  option {
    name  = "1 Cores"
    value = "1"
  }
  option {
    name  = "2 Cores"
    value = "2"
  }
  option {
    name  = "3 Cores"
    value = "3"
  }
  option {
    name  = "4 Cores"
    value = "4"
  }
}

data "coder_parameter" "memory" {
  name         = "memory"
  display_name = "Memory"
  description  = "The amount of memory in GB"
  default      = "2"
  icon         = "/icon/memory.svg"
  mutable      = true
  option {
    name  = "2 GB"
    value = "2"
  }
  option {
    name  = "4 GB"
    value = "4"
  }
  option {
    name  = "6 GB"
    value = "6"
  }
  option {
    name  = "8 GB"
    value = "8"
  }
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
  startup_script = <<-EOT
    set -e
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
  EOT

  metadata {
    display_name = "Load Average (Host)"
    key          = "load_host"
    # get load avg scaled by number of cores
    script   = <<EOT
      echo "`cat /proc/loadavg | awk '{ print $1 }'` `nproc`" | awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }
}

# code-server
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  icon         = "/icon/code.svg"
  url          = "http://localhost:13337?folder=/home/coder"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}

locals {
  workspace_tag    = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
  home_volume_name = "coder_${data.coder_workspace.me.id}_home"
}

resource "nomad_namespace" "coder_workspace" {
  name        = local.workspace_tag
  description = "Coder workspace"
  meta = {
    owner = data.coder_workspace.me.owner
  }
}

data "nomad_plugin" "hostpath" {
  plugin_id        = "hostpath"
  wait_for_healthy = true
}

resource "nomad_csi_volume" "home_volume" {
  depends_on = [data.nomad_plugin.hostpath]

  lifecycle {
    ignore_changes = all
  }
  plugin_id = "hostpath"
  volume_id = local.home_volume_name
  name      = local.home_volume_name
  namespace = nomad_namespace.coder_workspace.name

  capability {
    access_mode     = "single-node-writer"
    attachment_mode = "file-system"
  }

  mount_options {
    fs_type = "ext4"
  }
}

resource "nomad_job" "workspace" {
  count      = data.coder_workspace.me.start_count
  depends_on = [nomad_csi_volume.home_volume]
  jobspec = templatefile("${path.module}/workspace.nomad.tpl", {
    coder_workspace_owner = data.coder_workspace.me.owner
    coder_workspace_name  = data.coder_workspace.me.name
    workspace_tag         = local.workspace_tag
    cores                 = tonumber(data.coder_parameter.cpu.value)
    memory_mb             = tonumber(data.coder_parameter.memory.value * 1024)
    coder_init_script     = coder_agent.main.init_script
    coder_agent_token     = coder_agent.main.token
    workspace_name        = data.coder_workspace.me.name
    home_volume_name      = local.home_volume_name
  })
  deregister_on_destroy = true
  purge_on_destroy      = true
}

resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = nomad_job.workspace[0].id
  item {
    key   = "CPU (Cores)"
    value = data.coder_parameter.cpu.value
  }
  item {
    key   = "Memory (GiB)"
    value = data.coder_parameter.memory.value
  }
}
