terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

locals {
  username = data.coder_workspace.me.owner
}

data "coder_provisioner" "me" {
}

provider "docker" {
}

data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  arch                   = data.coder_provisioner.me.arch
  os                     = "linux"
  startup_script_timeout = 180
  startup_script         = <<-EOT
    set -e

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.11.0
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
  EOT
}

resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  url          = "http://localhost:13337/?folder=/home/${local.username}"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace.me.owner
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace.me.owner_id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_image" "main" {
  name = "coder-${data.coder_workspace.me.id}"
  build {
    context = "./build"
    build_args = {
      USER = local.username
    }
  }
  triggers = {
    dir_sha1 = sha1(join("", [for f in fileset(path.module, "build/*") : filesha1(f)]))
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = docker_image.main.name
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/${local.username}"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace.me.owner
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace.me.owner_id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}

// Rich parameters
// See: https://coder.com/docs/v2/latest/templates/parameters

data "coder_parameter" "project_id" {
  name         = "project_id"
  display_name = "My Project ID"
  icon         = "/emojis/1fab5.png"
  description  = "Specify the project ID to deploy in workspace."
  default      = "A1B2C3"
  mutable      = true
  validation {
    regex = "^[A-Z0-9]+$"
    error = "Project ID is incorrect"
  }

  order = 1
}

data "coder_parameter" "region" {
  name         = "region"
  display_name = "Region"
  icon         = "/emojis/1f30e.png"
  description  = "Select the region in which you would like to deploy your workspace."
  default      = "eu-helsinki"
  option {
    icon        = "/emojis/1f1fa-1f1f8.png"
    name        = "Pittsburgh"
    description = "Pittsburgh is a city in the Commonwealth of Pennsylvania and the county seat of Allegheny County."
    value       = "us-pittsburgh"
  }
  option {
    icon        = "/emojis/1f1eb-1f1ee.png"
    name        = "Helsinki"
    description = "Helsinki, the capital city of Finland, is renowned for its vibrant cultural scene, stunning waterfront architecture, and a harmonious blend of modernity and natural beauty."
    value       = "eu-helsinki"
  }
  option {
    icon        = "/emojis/1f1e6-1f1fa.png"
    name        = "Sydney"
    description = "Sydney, the largest city in Australia, captivates with its iconic Sydney Opera House, picturesque harbor, and diverse neighborhoods, making it a captivating blend of urban sophistication and coastal charm."
    value       = "ap-sydney"
  }

  order = 1
}

data "coder_parameter" "apps_dir" {
  name         = "apps_dir"
  display_name = "Apps Directory"
  icon         = "/emojis/1f9ba.png"
  type         = "string"
  description  = "Specify the directory to install project applications and tools."
  default      = "/var/apps"

  order = 2
}

data "coder_parameter" "worker_instances" {
  name         = "worker_instances"
  display_name = "Worker Instances"
  icon         = "/emojis/2697.png"
  type         = "number"
  description  = "Specify the number of worker instances to spawn."
  default      = "3"
  mutable      = true
  validation {
    min       = 3
    max       = 12
    monotonic = "increasing"
  }
  order = 2
}

data "coder_parameter" "security_groups" {
  name         = "security_groups"
  display_name = "Security Groups"
  icon         = "/emojis/26f4.png"
  type         = "list(string)"
  description  = "Select relevant security groups."
  mutable      = true
  default = jsonencode([
    "Web Server Security Group",
    "Database Security Group",
    "Backend Security Group"
  ])
  order = 2
}

data "coder_parameter" "docker_image" {
  name         = "docker_image"
  display_name = "Docker Image"
  mutable      = true
  type         = "string"
  description  = "Docker image for the development container"
  default      = "ghcr.io/coder/coder-preview:main"

  order = 3
}

data "coder_parameter" "command_line_args" {
  name         = "command_line_args"
  display_name = "Extra command line args"
  type         = "string"
  default      = ""
  description  = "Provide extra command line args for the startup script."
  mutable      = true
  order        = 80
}

data "coder_parameter" "enable_monitoring" {
  name         = "enable_monitoring"
  display_name = "Enable Workspace Monitoring"
  type         = "bool"
  description  = "This monitoring functionality empowers you to closely track the health and resource utilization of your instance in real-time."
  mutable      = true
  order        = 90
}

// Build options (ephemeral parameters)
// See: https://coder.com/docs/v2/latest/templates/parameters#ephemeral-parameters

data "coder_parameter" "pause-startup" {
  name         = "pause-startup"
  display_name = "Pause startup script"
  type         = "number"
  description  = "Pause the startup script (seconds)"
  default      = "1"
  mutable      = true
  ephemeral    = true
  validation {
    min = 0
    max = 300
  }

  order = 4
}

data "coder_parameter" "force-rebuild" {
  name         = "force-rebuild"
  display_name = "Force rebuild project"
  type         = "bool"
  description  = "Rebuild the workspace project"
  default      = "false"
  mutable      = true
  ephemeral    = true

  order = 4
}
