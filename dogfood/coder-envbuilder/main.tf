terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.0"
    }
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

locals {
  // These are cluster service addresses mapped to Tailscale nodes.
  // Ask #dogfood-admins for help.
  // NOTE: keep these up to date with those in ../dogfood/main.tf!
  docker_host = {
    ""              = "tcp://dogfood-ts-cdr-dev.tailscale.svc.cluster.local:2375"
    "us-pittsburgh" = "tcp://dogfood-ts-cdr-dev.tailscale.svc.cluster.local:2375"
    "eu-helsinki"   = "tcp://reinhard-hel-cdr-dev.tailscale.svc.cluster.local:2375"
    "ap-sydney"     = "tcp://wolfgang-syd-cdr-dev.tailscale.svc.cluster.local:2375"
    "sa-saopaulo"   = "tcp://oberstein-sao-cdr-dev.tailscale.svc.cluster.local:2375"
    "za-jnb"        = "tcp://greenhill-jnb-cdr-dev.tailscale.svc.cluster.local:2375"
  }

  envbuilder_repo = "ghcr.io/coder/envbuilder-preview"
  container_name  = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  // Envbuilder clones repos to /workspaces by default.
  repo_dir = "/workspaces/coder"
}

data "coder_parameter" "devcontainer_repo" {
  type        = "string"
  name        = "Devcontainer Repository"
  default     = "https://github.com/coder/coder"
  description = "Repo containing a devcontainer.json. This is only cloned once."
  mutable     = false
}

data "coder_parameter" "devcontainer_dir" {
  type        = "string"
  name        = "Devcontainer Directory"
  default     = "dogfood/coder/"
  description = "Directory containing a devcontainer.json relative to the repository root"
  mutable     = true
}

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
    icon  = "/emojis/1f1eb-1f1ee.png"
    name  = "Helsinki"
    value = "eu-helsinki"
  }
  option {
    icon  = "/emojis/1f1e6-1f1fa.png"
    name  = "Sydney"
    value = "ap-sydney"
  }
  option {
    icon  = "/emojis/1f1e7-1f1f7.png"
    name  = "São Paulo"
    value = "sa-saopaulo"
  }
  option {
    icon  = "/emojis/1f1ff-1f1e6.png"
    name  = "Johannesburg"
    value = "za-jnb"
  }
}

# This file is mounted as a Kubernetes secret on provisioner pods.
# It contains the required credentials for the envbuilder cache repo.
variable "envbuilder_cache_dockerconfigjson_path" {
  type      = string
  sensitive = true
}

provider "docker" {
  host = lookup(local.docker_host, data.coder_parameter.region.value)
  registry_auth {
    address     = "us-central1-docker.pkg.dev"
    config_file = pathexpand(var.envbuilder_cache_dockerconfigjson_path)
  }
}

provider "coder" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

module "slackme" {
  source           = "registry.coder.com/modules/slackme/coder"
  version          = "1.0.2"
  agent_id         = coder_agent.dev.id
  auth_provider_id = "slack"
}

module "dotfiles" {
  source   = "registry.coder.com/modules/dotfiles/coder"
  version  = "1.0.15"
  agent_id = coder_agent.dev.id
}

module "personalize" {
  source   = "registry.coder.com/modules/personalize/coder"
  version  = "1.0.2"
  agent_id = coder_agent.dev.id
}

module "code-server" {
  source                  = "registry.coder.com/modules/code-server/coder"
  version                 = "1.0.15"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  auto_install_extensions = true
}

module "jetbrains_gateway" {
  source         = "registry.coder.com/modules/jetbrains-gateway/coder"
  version        = "1.0.13"
  agent_id       = coder_agent.dev.id
  agent_name     = "dev"
  folder         = local.repo_dir
  jetbrains_ides = ["GO", "WS"]
  default        = "GO"
  latest         = true
}

module "filebrowser" {
  source   = "registry.coder.com/modules/filebrowser/coder"
  version  = "1.0.8"
  agent_id = coder_agent.dev.id
}

module "coder-login" {
  source   = "registry.coder.com/modules/coder-login/coder"
  version  = "1.0.15"
  agent_id = coder_agent.dev.id
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    OIDC_TOKEN : data.coder_workspace_owner.me.oidc_access_token,
  }
  startup_script_behavior = "blocking"

  # The following metadata blocks are optional. They are used to display
  # information about your workspace in the dashboard. You can remove them
  # if you don't want to display any information.
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
    display_name = "CPU Usage (Host)"
    key          = "cpu_usage_host"
    order        = 2
    script       = "coder stat cpu --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage (Host)"
    key          = "ram_usage_host"
    order        = 3
    script       = "coder stat mem --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Swap Usage (Host)"
    key          = "swap_usage_host"
    order        = 4
    script       = <<EOT
      #!/bin/bash
      echo "$(free -b | awk '/^Swap/ { printf("%.1f/%.1f", $3/1024.0/1024.0/1024.0, $2/1024.0/1024.0/1024.0) }') GiB"
    EOT
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Load Average (Host)"
    key          = "load_host"
    order        = 5
    # get load avg scaled by number of cores
    script   = <<EOT
      #!/bin/bash
      echo "`cat /proc/loadavg | awk '{ print $1 }'` `nproc`" | awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }

  metadata {
    display_name = "Disk Usage (Host)"
    key          = "disk_host"
    order        = 6
    script       = "coder stat disk --path /"
    interval     = 600
    timeout      = 10
  }

  metadata {
    display_name = "Word of the Day"
    key          = "word"
    order        = 7
    script       = <<EOT
      #!/bin/bash
      curl -o - --silent https://www.merriam-webster.com/word-of-the-day 2>&1 | awk ' $0 ~ "Word of the Day: [A-z]+" { print $5; exit }'
    EOT
    interval     = 86400
    timeout      = 5
  }

  startup_script = <<-EOT
    set -eux -o pipefail

    # Allow synchronization between scripts.
    trap 'touch /tmp/.coder-startup-script.done' EXIT

    # BUG: Kaniko does not symlink /run => /var/run properly, resulting in
    # /var/run/ owned by root:root
    # WORKAROUND: symlink it manually
    sudo ln -s /run /var/run
    # Start Docker service
    sudo service docker start

    # Chown /var/run/docker.sock as even though we are a member of the Docker group
    # it did not exist at the start of the workspace. This can be worked around with
    # `newgrp docker` but this is annoying to have to do manually.
    for attempt in $(seq 1 10); do
      if sudo docker info > /dev/null; then break; fi
      sleep 1
    done
    sudo chmod a+rw /var/run/docker.sock

    # Install playwright dependencies
    # We want to use the playwright version from site/package.json
    # Check if the directory exists At workspace creation as the coder_script runs in parallel so clone might not exist yet.
    while ! [[ -f "${local.repo_dir}/site/package.json" ]]; do
      sleep 1
    done
    cd "${local.repo_dir}/site" && pnpm install && pnpm playwright:install
  EOT
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

resource "docker_volume" "workspaces" {
  name = "coder-${data.coder_workspace.me.id}"
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

# This file is mounted as a Kubernetes secret on provisioner pods.
# It contains the required credentials for the envbuilder cache repo.
data "local_sensitive_file" "envbuilder_cache_dockerconfigjson" {
  filename = var.envbuilder_cache_dockerconfigjson_path
}

data "docker_registry_image" "envbuilder" {
  name = "${local.envbuilder_repo}:latest"
}

resource "docker_image" "envbuilder" {
  name          = "${local.envbuilder_repo}@${data.docker_registry_image.envbuilder.sha256_digest}"
  pull_triggers = [data.docker_registry_image.envbuilder.sha256_digest]
  keep_locally  = true
}

locals {
  cache_repo = "us-central1-docker.pkg.dev/coder-dogfood-v2/envbuilder-cache/coder-dogfood"
  envbuilder_env = {
    "CODER_AGENT_TOKEN" : coder_agent.dev.token,
    "CODER_AGENT_URL" : data.coder_workspace.me.access_url,
    "ENVBUILDER_GIT_USERNAME" : data.coder_external_auth.github.access_token,
    # "ENVBUILDER_GIT_URL" : data.coder_parameter.devcontainer_repo.value, # The provider sets this via the `git_url` property.
    "ENVBUILDER_DEVCONTAINER_DIR" : data.coder_parameter.devcontainer_dir.value,
    "ENVBUILDER_INIT_SCRIPT" : coder_agent.dev.init_script,
    "ENVBUILDER_FALLBACK_IMAGE" : "codercom/oss-dogfood:latest", # This image runs if builds fail
    "ENVBUILDER_PUSH_IMAGE" : "true",                            # Push the image to the remote cache
    # "ENVBUILDER_CACHE_REPO" : local.cache_repo, # The provider sets this via the `cache_repo` property.
    "ENVBUILDER_DOCKER_CONFIG_BASE64" : data.local_sensitive_file.envbuilder_cache_dockerconfigjson.content_base64,
    "USE_CAP_NET_ADMIN" : "true",
    # Set git commit details correctly
    "GIT_AUTHOR_NAME" : coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name),
    "GIT_AUTHOR_EMAIL" : data.coder_workspace_owner.me.email,
    "GIT_COMMITTER_NAME" : coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name),
    "GIT_COMMITTER_EMAIL" : data.coder_workspace_owner.me.email,
  }
}

# Check for the presence of a prebuilt image in the cache repo
# that we can use instead.
resource "envbuilder_cached_image" "cached" {
  count         = data.coder_workspace.me.start_count
  builder_image = docker_image.envbuilder.name
  git_url       = data.coder_parameter.devcontainer_repo.value
  cache_repo    = local.cache_repo
  extra_env     = local.envbuilder_env
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = envbuilder_cached_image.cached.0.image
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # CPU limits are unnecessary since Docker will load balance automatically
  memory  = 32768
  runtime = "sysbox-runc"
  # Use environment computed from the provider
  env = envbuilder_cached_image.cached.0.env
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  volumes {
    container_path = local.repo_dir
    volume_name    = docker_volume.workspaces.name
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

resource "coder_metadata" "container_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = coder_agent.dev.id
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
