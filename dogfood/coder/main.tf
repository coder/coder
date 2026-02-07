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

// This module is a terraform no-op. It contains 5mb worth of files to test
// Coder's behavior dealing with larger modules. This is included to test
// protobuf message size limits and the performance of module loading.
//
// In reality, modules might have accidental bloat from non-terraform files such
// as images & documentation.
module "large-5mb-module" {
  source = "git::https://github.com/coder/large-module.git"
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

  repo_base_dir  = data.coder_parameter.repo_base_dir.value == "~" ? "/home/coder" : replace(data.coder_parameter.repo_base_dir.value, "/^~\\//", "/home/coder/")
  repo_dir       = replace(try(module.git-clone[0].repo_dir, ""), "/^~\\//", "/home/coder/")
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

data "coder_workspace_preset" "pittsburgh" {
  name        = "Pittsburgh"
  default     = true
  description = "Development workspace hosted in United States with 2 prebuild instances"
  icon        = "/emojis/1f1fa-1f1f8.png"
  parameters = {
    (data.coder_parameter.region.name)                   = "us-pittsburgh"
    (data.coder_parameter.image_type.name)               = "codercom/oss-dogfood:latest"
    (data.coder_parameter.repo_base_dir.name)            = "~"
    (data.coder_parameter.res_mon_memory_threshold.name) = 80
    (data.coder_parameter.res_mon_volume_threshold.name) = 90
    (data.coder_parameter.res_mon_volume_path.name)      = "/home/coder"
  }
  prebuilds {
    instances = 2
  }
}

data "coder_workspace_preset" "cpt" {
  name        = "Cape Town"
  description = "Development workspace hosted in South Africa with 1 prebuild instance"
  icon        = "/emojis/1f1ff-1f1e6.png"
  parameters = {
    (data.coder_parameter.region.name)                   = "za-cpt"
    (data.coder_parameter.image_type.name)               = "codercom/oss-dogfood:latest"
    (data.coder_parameter.repo_base_dir.name)            = "~"
    (data.coder_parameter.res_mon_memory_threshold.name) = 80
    (data.coder_parameter.res_mon_volume_threshold.name) = 90
    (data.coder_parameter.res_mon_volume_path.name)      = "/home/coder"
  }
  prebuilds {
    instances = 1
  }
}

data "coder_workspace_preset" "falkenstein" {
  name        = "Falkenstein"
  description = "Development workspace hosted in Europe with 1 prebuild instance"
  icon        = "/emojis/1f1ea-1f1fa.png"
  parameters = {
    (data.coder_parameter.region.name)                   = "eu-helsinki"
    (data.coder_parameter.image_type.name)               = "codercom/oss-dogfood:latest"
    (data.coder_parameter.repo_base_dir.name)            = "~"
    (data.coder_parameter.res_mon_memory_threshold.name) = 80
    (data.coder_parameter.res_mon_volume_threshold.name) = 90
    (data.coder_parameter.res_mon_volume_path.name)      = "/home/coder"
  }
  prebuilds {
    instances = 1
  }
}

data "coder_workspace_preset" "sydney" {
  name        = "Sydney"
  description = "Development workspace hosted in Australia with 1 prebuild instance"
  icon        = "/emojis/1f1e6-1f1fa.png"
  parameters = {
    (data.coder_parameter.region.name)                   = "ap-sydney"
    (data.coder_parameter.image_type.name)               = "codercom/oss-dogfood:latest"
    (data.coder_parameter.repo_base_dir.name)            = "~"
    (data.coder_parameter.res_mon_memory_threshold.name) = 80
    (data.coder_parameter.res_mon_volume_threshold.name) = 90
    (data.coder_parameter.res_mon_volume_path.name)      = "/home/coder"
  }
  prebuilds {
    instances = 1
  }
}

data "coder_parameter" "repo_base_dir" {
  type        = "string"
  name        = "Coder Repository Base Directory"
  default     = "~"
  description = "The directory specified will be created (if missing) and [coder/coder](https://github.com/coder/coder) will be automatically cloned into [base directory]/coder 🪄."
  mutable     = true
}

data "coder_parameter" "image_type" {
  type        = "string"
  name        = "Coder Image"
  default     = "codercom/oss-dogfood:latest"
  description = "The Docker image used to run your workspace. Choose between nix and non-nix images."
  option {
    icon  = "/icon/coder.svg"
    name  = "Dogfood (Default)"
    value = "codercom/oss-dogfood:latest"
  }
  option {
    icon  = "/icon/nix.svg"
    name  = "Dogfood Nix (Experimental)"
    value = "codercom/oss-dogfood-nix:latest"
  }
}

locals {
  default_regions = {
    // keys should match group names
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

data "coder_parameter" "res_mon_volume_path" {
  type        = "string"
  name        = "Volume path"
  default     = "/home/coder"
  description = "The path monitored in resources monitoring to trigger notifications."
  mutable     = true
}

data "coder_parameter" "devcontainer_autostart" {
  type        = "bool"
  name        = "Automatically start devcontainer for coder/coder"
  default     = false
  description = "If enabled, a devcontainer will be automatically started for the [coder/coder](https://github.com/coder/coder) repository."
  mutable     = true
}

data "coder_parameter" "use_ai_bridge" {
  type        = "bool"
  name        = "Use AI Bridge"
  default     = true
  description = "If enabled, AI requests will be sent via AI Bridge."
  mutable     = true
}

# Only used if AI Bridge is disabled.
# dogfood/main.tf injects this value from a GH Actions secret;
# `coderd_template.dogfood` passes the value injected by .github/workflows/dogfood.yaml in `TF_VAR_CODER_DOGFOOD_ANTHROPIC_API_KEY`.
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
data "coder_task" "me" {}
data "coder_workspace_tags" "tags" {
  tags = {
    "cluster" : "dogfood-v2"
    "env" : "gke"
  }
}

data "coder_workspace_tags" "prebuild" {
  count = data.coder_workspace_owner.me.name == "prebuilds" ? 1 : 0
  tags = {
    "is_prebuild" = "true"
  }
}

data "coder_parameter" "ide_choices" {
  type        = "list(string)"
  name        = "Select IDEs"
  form_type   = "multi-select"
  mutable     = true
  description = "Choose one or more IDEs to enable in your workspace"
  default     = jsonencode(["vscode", "code-server", "cursor"])
  option {
    name  = "VS Code Desktop"
    value = "vscode"
    icon  = "/icon/code.svg"
  }
  option {
    name  = "code-server"
    value = "code-server"
    icon  = "/icon/code.svg"
  }
  option {
    name  = "VS Code Web"
    value = "vscode-web"
    icon  = "/icon/code.svg"
  }
  option {
    name  = "JetBrains IDEs"
    value = "jetbrains"
    icon  = "/icon/jetbrains.svg"
  }
  option {
    name  = "Cursor"
    value = "cursor"
    icon  = "/icon/cursor.svg"
  }
  option {
    name  = "Windsurf"
    value = "windsurf"
    icon  = "/icon/windsurf.svg"
  }
  option {
    name  = "Zed"
    value = "zed"
    icon  = "/icon/zed.svg"
  }
}

data "coder_parameter" "vscode_channel" {
  count       = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode") ? 1 : 0
  type        = "string"
  name        = "VS Code Desktop channel"
  description = "Choose the VS Code Desktop channel"
  mutable     = true
  default     = "stable"
  option {
    value = "stable"
    name  = "Stable"
    icon  = "/icon/code.svg"
  }
  option {
    value = "insiders"
    name  = "Insiders"
    icon  = "/icon/code-insiders.svg"
  }
}

module "slackme" {
  count            = data.coder_workspace.me.start_count
  source           = "dev.registry.coder.com/coder/slackme/coder"
  version          = "1.0.33"
  agent_id         = coder_agent.dev.id
  auth_provider_id = "slack"
}

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/dotfiles/coder"
  version  = "1.2.3"
  agent_id = coder_agent.dev.id
}

module "git-config" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/git-config/coder"
  version  = "1.0.32"
  agent_id = coder_agent.dev.id
  # If you prefer to commit with a different email, this allows you to do so.
  allow_email_change = true
}

module "git-clone" {
  count             = data.coder_workspace.me.start_count
  source            = "dev.registry.coder.com/coder/git-clone/coder"
  version           = "1.2.3"
  agent_id          = coder_agent.dev.id
  url               = "https://github.com/coder/coder"
  base_dir          = local.repo_base_dir
  post_clone_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail
    coder exp sync start git-clone
    coder exp sync complete git-clone
  EOT
}

module "personalize" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/personalize/coder"
  version  = "1.0.32"
  agent_id = coder_agent.dev.id
}

module "mux" {
  count        = data.coder_workspace.me.start_count
  source       = "registry.coder.com/coder/mux/coder"
  version      = "1.0.8"
  agent_id     = coder_agent.dev.id
  subdomain    = true
  display_name = "Mux"
  add-project  = local.repo_dir
}

module "code-server" {
  count                   = contains(jsondecode(data.coder_parameter.ide_choices.value), "code-server") ? data.coder_workspace.me.start_count : 0
  source                  = "dev.registry.coder.com/coder/code-server/coder"
  version                 = "1.4.2"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  auto_install_extensions = true
  group                   = "Web Editors"
}

module "vscode-web" {
  count                   = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode-web") ? data.coder_workspace.me.start_count : 0
  source                  = "dev.registry.coder.com/coder/vscode-web/coder"
  version                 = "1.4.3"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  extensions              = ["github.copilot"]
  auto_install_extensions = true # will install extensions from the repos .vscode/extensions.json file
  accept_license          = true
  group                   = "Web Editors"
}

module "jetbrains" {
  count         = contains(jsondecode(data.coder_parameter.ide_choices.value), "jetbrains") ? data.coder_workspace.me.start_count : 0
  source        = "dev.registry.coder.com/coder/jetbrains/coder"
  version       = "1.3.0"
  agent_id      = coder_agent.dev.id
  agent_name    = "dev"
  folder        = local.repo_dir
  major_version = "latest"
  tooltip       = "You need to [install JetBrains Toolbox](https://coder.com/docs/user-guides/workspace-access/jetbrains/toolbox) to use this app."
}

module "filebrowser" {
  count      = data.coder_workspace.me.start_count
  source     = "dev.registry.coder.com/coder/filebrowser/coder"
  version    = "1.1.4"
  agent_id   = coder_agent.dev.id
  agent_name = "dev"
}

module "coder-login" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/coder/coder-login/coder"
  version  = "1.1.1"
  agent_id = coder_agent.dev.id
}

module "cursor" {
  count    = contains(jsondecode(data.coder_parameter.ide_choices.value), "cursor") ? data.coder_workspace.me.start_count : 0
  source   = "dev.registry.coder.com/coder/cursor/coder"
  version  = "1.4.0"
  agent_id = coder_agent.dev.id
  folder   = local.repo_dir
}

module "windsurf" {
  count    = contains(jsondecode(data.coder_parameter.ide_choices.value), "windsurf") ? data.coder_workspace.me.start_count : 0
  source   = "dev.registry.coder.com/coder/windsurf/coder"
  version  = "1.3.0"
  agent_id = coder_agent.dev.id
  folder   = local.repo_dir
}

module "zed" {
  count      = contains(jsondecode(data.coder_parameter.ide_choices.value), "zed") ? data.coder_workspace.me.start_count : 0
  source     = "dev.registry.coder.com/coder/zed/coder"
  version    = "1.1.4"
  agent_id   = coder_agent.dev.id
  agent_name = "dev"
  folder     = local.repo_dir
}

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  version  = ">= 1.0.0"
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

  display_apps {
    vscode          = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode") && try(data.coder_parameter.vscode_channel[0].value, "stable") == "stable"
    vscode_insiders = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode") && try(data.coder_parameter.vscode_channel[0].value, "stable") == "insiders"
  }

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
    display_name = "/home Usage"
    key          = "home_usage"
    order        = 2
    script       = "sudo du -sh /home/coder | awk '{print $1}'"
    interval     = 3600 # 1h to avoid thrashing disk
    timeout      = 60   # Longer than this is likely problematic
  }

  metadata {
    display_name = "/var/lib/docker Usage"
    key          = "var_lib_docker_usage"
    order        = 3
    script       = "sudo du -sh /var/lib/docker | awk '{print $1}'"
    interval     = 3600 # 1h to avoid thrashing disk
    timeout      = 60   # Longer than this is likely problematic
  }

  metadata {
    display_name = "Word of the Day"
    key          = "word"
    order        = 4
    script       = <<EOT
      #!/usr/bin/env bash
      curl -o - --silent https://www.merriam-webster.com/word-of-the-day 2>&1 | awk ' $0 ~ "Word of the Day: [A-z]+" { print $5; exit }'
    EOT
    interval     = 86400
    timeout      = 5
  }

  resources_monitoring {
    memory {
      enabled   = true
      threshold = data.coder_parameter.res_mon_memory_threshold.value
    }
    volume {
      enabled   = true
      threshold = data.coder_parameter.res_mon_volume_threshold.value
      path      = data.coder_parameter.res_mon_volume_path.value
    }
    volume {
      enabled   = true
      threshold = data.coder_parameter.res_mon_volume_threshold.value
      path      = "/var/lib/docker"
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

    # Increase the shutdown timeout of the docker service for improved cleanup.
    # The 240 was picked as it's lower than the 300 seconds we set for the
    # container shutdown grace period.
    sudo sh -c 'jq ". += {\"shutdown-timeout\": 240}" /etc/docker/daemon.json > /tmp/daemon.json.new && mv /tmp/daemon.json.new /etc/docker/daemon.json'
    # Start Docker service
    sudo service docker start
  EOT

  shutdown_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Clean up the Go build cache to prevent the home volume from
    # accumulating waste and growing too large.
    go clean -cache

    # Clean up the coder build directory as this can get quite large
    rm -rf "${local.repo_dir}/build"

    # Clean up the unused resources to keep storage usage low.
    #
    # WARNING! This will remove:
    #   - all stopped containers
    #   - all networks not used by at least one container
    #   - all images without at least one container associated to them
    #   - all build cache
    docker system prune -a -f

    # Stop the Docker service to prevent errors during workspace destroy.
    sudo service docker stop
  EOT
}

resource "coder_script" "install-deps" {
  agent_id           = coder_agent.dev.id
  display_name       = "Installing Dependencies"
  run_on_start       = true
  start_blocks_login = false
  script             = <<EOT
    #!/usr/bin/env bash
    set -euo pipefail

    trap 'coder exp sync complete install-deps' EXIT
    coder exp sync want install-deps git-clone
    coder exp sync start install-deps

    # Install playwright dependencies
    # We want to use the playwright version from site/package.json
    cd "${local.repo_dir}" && make clean
    cd "${local.repo_dir}/site" && pnpm install
  EOT
}

resource "coder_devcontainer" "coder" {
  count            = data.coder_parameter.devcontainer_autostart.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.dev.id
  workspace_folder = local.repo_dir
}

# Add a cost so we get some quota usage in dev.coder.com
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

resource "coder_metadata" "docker_volume" {
  resource_id = docker_volume.docker_volume.id
  hide        = true # Hide it as it is not useful to see in the UI.
}

resource "docker_volume" "docker_volume" {
  name = "coder-${data.coder_workspace.me.id}-docker"
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
  name = data.coder_parameter.image_type.value
}

resource "docker_image" "dogfood" {
  name = "${data.coder_parameter.image_type.value}@${data.docker_registry_image.dogfood.sha256_digest}"
  pull_triggers = [
    data.docker_registry_image.dogfood.sha256_digest,
    sha1(join("", [for f in fileset(path.module, "files/*") : filesha1(f)])),
    filesha1("Dockerfile"),
    filesha1("nix.hash"),
  ]
  keep_locally = true
}

resource "docker_container" "workspace" {
  lifecycle {
    // Ignore changes that would invalidate prebuilds
    ignore_changes = [
      name,
      hostname,
      labels,
      env,
      entrypoint
    ]
  }
  count = data.coder_workspace.me.start_count
  image = docker_image.dogfood.name
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  # CPU limits are unnecessary since Docker will load balance automatically
  memory  = data.coder_workspace_owner.me.name == "code-asher" ? 65536 : 32768
  runtime = "sysbox-runc"

  # Ensure the workspace is given time to:
  # - Execute shutdown scripts
  # - Stop the in workspace Docker daemon
  # - Stop the container, especially when using devcontainers,
  #   deleting the overlay filesystem can take a while.
  destroy_grace_seconds = 300
  stop_timeout          = 300
  stop_signal           = "SIGINT"

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
    "USE_CAP_NET_ADMIN=true",
    "CODER_PROC_PRIO_MGMT=1",
    "CODER_PROC_OOM_SCORE=10",
    "CODER_PROC_NICE_SCORE=1",
    "CODER_AGENT_DEVCONTAINERS_ENABLE=1",
    "CODER_AGENT_SOCKET_SERVER_ENABLED=true",
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
  volumes {
    container_path = "/var/lib/docker/"
    volume_name    = docker_volume.docker_volume.name
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
  item {
    key   = "ai_task"
    value = data.coder_task.me.enabled ? "yes" : "no"
  }
}

locals {
  claude_system_prompt = <<-EOT
    -- Framing --
    You are a helpful Coding assistant. Aim to autonomously investigate
    and solve issues the user gives you and test your work, whenever possible.

    Avoid shortcuts like mocking tests. When you get stuck, you can ask the user
    but opt for autonomy.

    -- Tool Selection --
    - playwright: previewing your changes after you made them
      to confirm it worked as expected
    -	Built-in tools - use for everything else:
      (file operations, git commands, builds & installs, one-off shell commands)

    -- Workflow --
    When starting new work:
    1. If given a GitHub issue URL, use the `gh` CLI to read the full issue details with `gh issue view <issue-number>`.
    2. Create a feature branch for the work using a descriptive name based on the issue or task.
       Example: `git checkout -b fix/issue-123-oauth-error` or `git checkout -b feat/add-dark-mode`
    3. Proceed with implementation following the CLAUDE.md guidelines.

    -- Context --
    There is an existing application in the current directory.
    Be sure to read CLAUDE.md before making any changes.

    This is a real-world production application. As such, make sure to think carefully, use TODO lists, and plan carefully before making changes.
  EOT
}

resource "coder_script" "boundary_config_setup" {
  agent_id     = coder_agent.dev.id
  display_name = "Boundary Setup Configuration"
  run_on_start = true

  script = <<-EOF
    #!/bin/sh

    trap 'coder exp sync complete boundary-config-setup' EXIT
    coder exp sync start boundary-config-setup

    mkdir -p ~/.config/coder_boundary
    echo '${base64encode(file("${path.module}/boundary-config.yaml"))}' | base64 -d > ~/.config/coder_boundary/config.yaml
    chmod 600 ~/.config/coder_boundary/config.yaml
  EOF
}

module "claude-code" {
  count               = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.7.2"
  enable_boundary     = true
  agent_id            = coder_agent.dev.id
  workdir             = local.repo_dir
  claude_code_version = "latest"
  model               = "opus"
  order               = 999
  claude_api_key      = data.coder_parameter.use_ai_bridge.value ? data.coder_workspace_owner.me.session_token : var.anthropic_api_key
  agentapi_version    = "latest"

  system_prompt       = local.claude_system_prompt
  ai_prompt           = data.coder_task.me.prompt
  post_install_script = <<-EOT
    cd $HOME/coder
    claude mcp add playwright npx -- @playwright/mcp@latest --headless --isolated --no-sandbox
  EOT
}

resource "coder_ai_task" "task" {
  count  = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  app_id = module.claude-code[count.index].task_app_id
}

resource "coder_app" "develop_sh" {
  count        = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  agent_id     = coder_agent.dev.id
  slug         = "develop-sh"
  display_name = "develop.sh"
  icon         = "${data.coder_workspace.me.access_url}/emojis/1f4bb.png" // 💻
  command      = "screen -x develop_sh"
  share        = "authenticated"
  subdomain    = true
  open_in      = "tab"
  order        = 0
}

resource "coder_script" "develop_sh" {
  count              = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  display_name       = "develop.sh"
  agent_id           = coder_agent.dev.id
  run_on_start       = true
  start_blocks_login = false
  icon               = "${data.coder_workspace.me.access_url}/emojis/1f4bb.png" // 💻
  script             = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    trap 'coder exp sync complete develop-sh' EXIT
    coder exp sync want develop-sh install-deps
    coder exp sync start develop-sh

    cd "${local.repo_dir}" && screen -dmS develop_sh /bin/sh -c 'while true; do ./scripts/develop.sh --; echo "develop.sh exited with code $? restarting in 30s"; sleep 30; done'
  EOT
}

resource "coder_app" "preview" {
  count        = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  agent_id     = coder_agent.dev.id
  slug         = "preview"
  display_name = "Preview"
  icon         = "${data.coder_workspace.me.access_url}/emojis/1f50e.png" // 🔎
  url          = "http://localhost:8080"
  share        = "authenticated"
  subdomain    = true
  open_in      = "tab"
  order        = 1
  healthcheck {
    url       = "http://localhost:8080/healthz"
    interval  = 5
    threshold = 15
  }
}
