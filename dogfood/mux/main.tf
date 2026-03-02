terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

locals {
  // These are cluster service addresses mapped to Tailscale nodes. Ask Dean or
  // Kyle for help.
  docker_host = {
    ""              = "tcp://rubinsky-pit-cdr-dev.tailscale.svc.cluster.local:2375"
    "us-pittsburgh" = "tcp://rubinsky-pit-cdr-dev.tailscale.svc.cluster.local:2375"
    // For legacy reasons, this host is labelled `eu-helsinki` but it's
    // actually in Germany now.
    "eu-helsinki" = "tcp://katerose-fsn-cdr-dev.tailscale.svc.cluster.local:2375"
    "ap-sydney"   = "tcp://wolfgang-syd-cdr-dev.tailscale.svc.cluster.local:2375"
    "za-cpt"      = "tcp://schonkopf-cpt-cdr-dev.tailscale.svc.cluster.local:2375"
  }

  repo_dir       = replace(module.git-clone[0].repo_dir, "/^~\\//", "/home/coder/")
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

locals {
  default_regions = {
    // Keys should match group names.
    "north-america" : "us-pittsburgh"
    "europe" : "eu-helsinki"
    "australia" : "ap-sydney"
    "africa" : "za-cpt"
  }

  user_groups = data.coder_workspace_owner.me.groups
  user_region = coalescelist([
    for g in local.user_groups :
    local.default_regions[g] if contains(keys(local.default_regions), g)
  ], ["us-pittsburgh"])[0]
}

data "coder_parameter" "region" {
  type    = "string"
  name    = "Region"
  icon    = "/emojis/1f30e.png"
  default = local.user_region
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

data "coder_parameter" "use_ai_bridge" {
  type        = "bool"
  name        = "Use AI Bridge"
  default     = true
  description = "If enabled, AI requests will be sent via AI Bridge."
  mutable     = true
}

data "coder_parameter" "res_mon_memory_threshold" {
  type        = "number"
  name        = "Memory usage threshold"
  default     = 80
  description = "The memory usage threshold used in resources monitoring to trigger notifications."
  mutable     = true
  validation {
    min = 0
    max = 100
  }
}

data "coder_parameter" "res_mon_volume_threshold" {
  type        = "number"
  name        = "Volume usage threshold"
  default     = 90
  description = "The volume usage threshold used in resources monitoring to trigger notifications."
  mutable     = true
  validation {
    min = 0
    max = 100
  }
}

# Only used if AI Bridge is disabled.
variable "anthropic_api_key" {
  type        = string
  description = "The API key used to authenticate with the Anthropic API, if AI Bridge is disabled."
  default     = ""
  sensitive   = true
}

provider "docker" {
  host = lookup(local.docker_host, data.coder_parameter.region.value)
}

provider "coder" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}
data "coder_workspace_tags" "tags" {
  tags = {
    "cluster" : "dogfood-v2"
    "env" : "gke"
  }
}

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/git-clone/coder"
  version  = "1.2.3"
  agent_id = coder_agent.dev.id
  url      = "https://github.com/coder/mux"
  base_dir = "/home/coder"
}

module "mux" {
  count        = data.coder_workspace.me.start_count
  source       = "registry.coder.com/coder/mux/coder"
  version      = "1.1.0"
  agent_id     = coder_agent.dev.id
  subdomain    = true
  display_name = "Mux"
}

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/dotfiles/coder"
  version  = "1.3.0"
  agent_id = coder_agent.dev.id
}

module "git-config" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/git-config/coder"
  version  = "1.0.33"
  agent_id = coder_agent.dev.id
  # If you prefer to commit with a different email, this allows you to do so.
  allow_email_change = true
}

module "personalize" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/personalize/coder"
  version  = "1.0.32"
  agent_id = coder_agent.dev.id
}

module "coder-login" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/coder-login/coder"
  version  = "1.1.1"
  agent_id = coder_agent.dev.id
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = merge(
    {
      OIDC_TOKEN : data.coder_workspace_owner.me.oidc_access_token,
    },
    data.coder_parameter.use_ai_bridge.value ? {
      ANTHROPIC_BASE_URL : "https://dev.coder.com/api/v2/aibridge/anthropic",
      ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token,
    } : {}
  )
  startup_script_behavior = "blocking"

  # Hide all built-in IDE apps — Mux is the IDE.
  display_apps {}

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

  resources_monitoring {
    memory {
      enabled   = true
      threshold = data.coder_parameter.res_mon_memory_threshold.value
    }
    volume {
      enabled   = true
      threshold = data.coder_parameter.res_mon_volume_threshold.value
      path      = "/home/coder"
    }
  }

  startup_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Allow other scripts to wait for agent startup.
    function cleanup() {
      coder exp sync complete agent-startup
      # Some folks will also use this for their personalize scripts.
      touch /tmp/.coder-startup-script.done
    }
    trap cleanup EXIT
    coder exp sync start agent-startup

    # Authenticate GitHub CLI
    if ! gh auth status >/dev/null 2>&1; then
      echo "Logging into GitHub CLI…"
      coder external-auth access-token github | gh auth login --hostname github.com --with-token
    else
      echo "Already logged into GitHub CLI."
    fi

    # Configure Mux GitHub owner login for browser access (skip if
    # already set). See: https://mux.coder.com/config/server-access
    if [ ! -f ~/.mux/config.json ] || ! jq -e '.serverAuthGithubOwner' ~/.mux/config.json >/dev/null 2>&1; then
      GH_USER=$(gh api user --jq .login 2>/dev/null || true)
      if [ -n "$GH_USER" ]; then
        mkdir -p ~/.mux
        if [ -f ~/.mux/config.json ]; then
          jq --arg owner "$GH_USER" '. + {serverAuthGithubOwner: $owner}' ~/.mux/config.json > /tmp/mux-config.json && mv /tmp/mux-config.json ~/.mux/config.json
        else
          jq -n --arg owner "$GH_USER" '{serverAuthGithubOwner: $owner}' > ~/.mux/config.json
        fi
        echo "Configured Mux GitHub owner login: $GH_USER"
      fi
    fi
  EOT
}

# Add a cost so we get some quota usage in dev.coder.com.
resource "coder_metadata" "home_volume" {
  resource_id = docker_volume.home_volume.id
  daily_cost  = 1
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

data "docker_registry_image" "dogfood" {
  name = "codercom/oss-dogfood:latest"
}

resource "docker_image" "dogfood" {
  name          = "codercom/oss-dogfood:latest@${data.docker_registry_image.dogfood.sha256_digest}"
  pull_triggers = [data.docker_registry_image.dogfood.sha256_digest]
  keep_locally  = true
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = docker_image.dogfood.name
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  # CPU limits are unnecessary since Docker will load balance automatically.
  memory = 16384

  env = ["CODER_AGENT_TOKEN=${coder_agent.dev.token}"]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
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

resource "coder_metadata" "container_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = docker_container.workspace[0].id
  item {
    key   = "memory"
    value = docker_container.workspace[0].memory
  }
  item {
    key   = "region"
    value = data.coder_parameter.region.option[index(data.coder_parameter.region.option.*.value, data.coder_parameter.region.value)].name
  }
}
