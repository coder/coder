terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.6"
    }
  }
}

locals {
  // These are cluster service addresses mapped to Tailscale nodes.
  // Ask Dean or Kyle for help.
  docker_host = {
    ""              = "tcp://rubinsky-pit-cdr-dev.tailscale.svc.cluster.local:2375"
    "us-pittsburgh" = "tcp://rubinsky-pit-cdr-dev.tailscale.svc.cluster.local:2375"
    // For legacy reasons, this host is labelled `eu-helsinki` but it's
    // actually in Germany now.
    "eu-helsinki" = "tcp://katerose-fsn-cdr-dev.tailscale.svc.cluster.local:2375"
    "ap-sydney"   = "tcp://wolfgang-syd-cdr-dev.tailscale.svc.cluster.local:2375"
    "za-cpt"      = "tcp://schonkopf-cpt-cdr-dev.tailscale.svc.cluster.local:2375"
  }

  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

# --- Parameters ---

data "coder_parameter" "region" {
  type    = "string"
  name    = "Region"
  icon    = "/emojis/1f30e.png"
  default = "us-pittsburgh"
  option {
    icon  = "/emojis/1f1fa-1f1f8.png"
    name  = "Pittsburgh"
    value = "us-pittsburgh"
  }
  option {
    icon = "/emojis/1f1e9-1f1ea.png"
    name = "Falkenstein"
    // For legacy reasons, this host is labelled `eu-helsinki` but it's
    // actually in Germany now.
    value = "eu-helsinki"
  }
  option {
    icon  = "/emojis/1f1e6-1f1fa.png"
    name  = "Sydney"
    value = "ap-sydney"
  }
  option {
    icon  = "/emojis/1f1ff-1f1e6.png"
    name  = "Cape Town"
    value = "za-cpt"
  }
}

data "coder_parameter" "repo" {
  type        = "string"
  name        = "Android Repository"
  default     = "https://github.com/coder/coder-mobile-android"
  description = "The Android project repository to clone into the workspace."
  mutable     = true
}

# --- Presets ---

data "coder_workspace_preset" "pittsburgh" {
  name        = "Pittsburgh"
  default     = true
  description = "Android development workspace hosted in United States"
  icon        = "/emojis/1f1fa-1f1f8.png"
  parameters = {
    (data.coder_parameter.region.name) = "us-pittsburgh"
    (data.coder_parameter.repo.name)   = "https://github.com/coder/coder-mobile-android"
  }
  prebuilds {
    instances = 0
  }
}

# --- Providers ---

provider "docker" {
  host = lookup(local.docker_host, data.coder_parameter.region.value)
}

provider "coder" {}

# --- Data Sources ---

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace_tags" "tags" {
  tags = {
    "cluster" : "dogfood-v2"
    "env" : "gke"
  }
}

# --- Docker Resources ---

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
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

data "docker_registry_image" "android" {
  name = "codercom/oss-dogfood-android:latest"
}

resource "docker_image" "android" {
  name = "codercom/oss-dogfood-android:latest@${data.docker_registry_image.android.sha256_digest}"
  pull_triggers = [
    data.docker_registry_image.android.sha256_digest,
    filesha1("Dockerfile"),
  ]
  keep_locally = true
}

resource "docker_container" "workspace" {
  lifecycle {
    ignore_changes = [
      name,
      hostname,
      labels,
      env,
      entrypoint,
    ]
  }
  count = data.coder_workspace.me.start_count
  image = docker_image.android.name
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  # 32 GB memory for Android builds and emulator.
  memory  = 32768
  runtime = "sysbox-runc"
  dns     = ["1.1.1.1"]

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  capabilities {
    add = ["CAP_NET_ADMIN", "CAP_SYS_NICE"]
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
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

# --- Coder Agent ---

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = "/home/coder"

  startup_script_behavior = "blocking"

  display_apps {
    vscode = true
  }

  env = {
    GITHUB_TOKEN = data.coder_external_auth.github.access_token
  }

  metadata {
    display_name = "CPU Usage"
    key          = "cpu_usage"
    order        = 0
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "ram_usage"
    order        = 1
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "/home Usage"
    key          = "home_usage"
    order        = 2
    script       = "sudo du -sh /home/coder | awk '{print $1}'"
    interval     = 3600 # 1h to avoid thrashing disk
    timeout      = 60   # Longer than this is likely problematic
  }

  startup_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Start Docker daemon if the socket is available (sysbox-runc
    # provides nested Docker support for emulator images, etc.).
    if command -v dockerd &>/dev/null; then
      sudo service docker start || true
    fi

    # Start the ADB server so devices/emulators are discoverable
    # as soon as the workspace is ready.
    if command -v adb &>/dev/null; then
      adb start-server || true
    fi

    # Print environment info for debugging build issues.
    echo "=== Android SDK ==="
    if [ -n "$${ANDROID_HOME:-}" ]; then
      echo "ANDROID_HOME=$ANDROID_HOME"
      "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" --list_installed 2>/dev/null | head -20 || true
    else
      echo "ANDROID_HOME is not set"
    fi

    echo "=== Go Version ==="
    go version || echo "Go is not installed"
  EOT
}

# --- Metadata ---

resource "coder_metadata" "home_volume" {
  resource_id = docker_volume.home_volume.id
  daily_cost  = 1
}

resource "coder_metadata" "container_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = docker_container.workspace[0].id
  item {
    key   = "memory"
    value = docker_container.workspace[0].memory
  }
  item {
    key   = "runtime"
    value = docker_container.workspace[0].runtime
  }
  item {
    key   = "region"
    value = data.coder_parameter.region.option[index(data.coder_parameter.region.option.*.value, data.coder_parameter.region.value)].name
  }
}

# --- Registry Modules ---

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/git-clone/coder"
  version  = "1.0.27"
  agent_id = coder_agent.dev.id
  url      = data.coder_parameter.repo.value
}

module "git-config" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/git-config/coder"
  version  = "1.0.16"
  agent_id = coder_agent.dev.id
}

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/dotfiles/coder"
  version  = "1.4.0"
  agent_id = coder_agent.dev.id
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/code-server/coder"
  version  = "1.2.0"
  agent_id = coder_agent.dev.id
}

# TODO: Add an android-studio coder_app when Projector or a web-based
# Android Studio solution is ready. It would serve on localhost:8080
# with subdomain = true for in-browser IDE access.
