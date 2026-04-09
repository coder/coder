terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.35.0"
    }
  }
}

// Included to test protobuf message size limits and the performance
// of module loading. Mirrors the same stress-test in the Docker
// dogfood template.
module "large-5mb-module" {
  source = "git::https://github.com/coder/large-module.git"
}

locals {
  repo_base_dir = data.coder_parameter.repo_base_dir.value == "~" ? "/home/coder" : replace(data.coder_parameter.repo_base_dir.value, "/^~\\//", "/home/coder/")
  repo_dir      = replace(try(module.git-clone[0].repo_dir, ""), "/^~\\//", "/home/coder/")

  // Derive a stable per-workspace hour and minute from the workspace
  // ID so that cache cleanup crons don't all hit the filesystem at
  // once.
  cache_cleanup_hour   = parseint(substr(data.coder_workspace.me.id, 0, 2), 16) % 24
  cache_cleanup_minute = parseint(substr(data.coder_workspace.me.id, 2, 2), 16) % 60

  workspace_labels = {
    "app.kubernetes.io/name"     = "coder-workspace"
    "app.kubernetes.io/instance" = "coder-ws-${data.coder_workspace.me.id}"
    "app.kubernetes.io/part-of"  = "coder"
    "com.coder.resource"         = "true"
    "com.coder.workspace.id"     = data.coder_workspace.me.id
    "com.coder.workspace.name"   = data.coder_workspace.me.name
    "com.coder.user.id"          = data.coder_workspace_owner.me.id
    "com.coder.user.username"    = data.coder_workspace_owner.me.name
  }
}

variable "namespace" {
  type        = string
  description = "The Kubernetes namespace for workspaces."
  default     = "coder-workspaces"
}

# Only used if AI Bridge is disabled.
# dogfood/main.tf injects this value from a GH Actions secret.
variable "anthropic_api_key" {
  type        = string
  description = "The API key used to authenticate with the Anthropic API, if AI Bridge is disabled."
  default     = ""
  sensitive   = true
}

# ---------------------------------------------------------------------------
# Providers & data sources
# ---------------------------------------------------------------------------

provider "coder" {}

provider "kubernetes" {
  # When the provisioner runs in-cluster the provider auto-discovers
  # the service account credentials.
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}
data "coder_task" "me" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace_tags" "tags" {
  tags = {
    "cluster" = "dogfood-v2"
    "env"     = "eks"
  }
}

data "coder_workspace_tags" "prebuild" {
  count = data.coder_workspace_owner.me.name == "prebuilds" ? 1 : 0
  tags = {
    "is_prebuild" = "true"
  }
}

# ---------------------------------------------------------------------------
# Prebuilds
# ---------------------------------------------------------------------------

data "coder_workspace_preset" "default" {
  name        = "Default"
  default     = true
  description = "Development workspace on Kubernetes with disk quotas"
  parameters = {
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

# ---------------------------------------------------------------------------
# Parameters
# ---------------------------------------------------------------------------

data "coder_parameter" "image_type" {
  type        = "string"
  name        = "Coder Image"
  default     = "codercom/oss-dogfood:latest"
  description = "The Docker image used to run your workspace."
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

data "coder_parameter" "home_disk_size" {
  name         = "home_disk_size"
  display_name = "Home Disk (GB)"
  description  = "Size of the persistent volume for /home/coder."
  icon         = "/emojis/1f4be.png"
  type         = "number"
  default      = "50"
  mutable      = false
  validation {
    min = 10
    max = 200
  }
}

data "coder_parameter" "docker_disk_size" {
  name         = "docker_disk_size"
  display_name = "Docker Cache Disk (GB)"
  description  = "Size of the persistent volume for the Docker cache."
  icon         = "/emojis/1f4be.png"
  type         = "number"
  default      = "50"
  mutable      = false
  validation {
    min = 10
    max = 200
  }
}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU Cores"
  description  = "CPU cores allocated to the workspace."
  type         = "number"
  default      = "8"
  mutable      = true
  option {
    name  = "4 Cores"
    value = "4"
  }
  option {
    name  = "8 Cores"
    value = "8"
  }
  option {
    name  = "16 Cores"
    value = "16"
  }
}

data "coder_parameter" "memory" {
  name         = "memory"
  display_name = "Memory (GB)"
  description  = "Memory allocated to the workspace."
  type         = "number"
  default      = "32"
  mutable      = true
  option {
    name  = "16 GB"
    value = "16"
  }
  option {
    name  = "32 GB"
    value = "32"
  }
  option {
    name  = "64 GB"
    value = "64"
  }
}

data "coder_parameter" "repo_base_dir" {
  type        = "string"
  name        = "Coder Repository Base Directory"
  default     = "~"
  description = "The directory specified will be created (if missing) and [coder/coder](https://github.com/coder/coder) will be automatically cloned into [base directory]/coder 🪄."
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

# ---------------------------------------------------------------------------
# Persistent Volumes
# ---------------------------------------------------------------------------

resource "kubernetes_persistent_volume_claim_v1" "home" {
  metadata {
    name      = "coder-${data.coder_workspace.me.id}-home"
    namespace = var.namespace
    labels    = local.workspace_labels
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${data.coder_parameter.home_disk_size.value}Gi"
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim_v1" "docker" {
  metadata {
    name      = "coder-${data.coder_workspace.me.id}-docker"
    namespace = var.namespace
    labels    = local.workspace_labels
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "${data.coder_parameter.docker_disk_size.value}Gi"
      }
    }
  }
}

# Quota: cost scales with chosen disk size so users with larger
# budgets can provision more or bigger workspaces.
resource "coder_metadata" "home_volume" {
  resource_id = kubernetes_persistent_volume_claim_v1.home.id
  daily_cost  = data.coder_parameter.home_disk_size.value
}

resource "coder_metadata" "docker_volume" {
  resource_id = kubernetes_persistent_volume_claim_v1.docker.id
  daily_cost  = data.coder_parameter.docker_disk_size.value
  hide        = true
}

# ---------------------------------------------------------------------------
# Agent
# ---------------------------------------------------------------------------

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
      OPENAI_BASE_URL : "https://dev.coder.com/api/v2/aibridge/openai/v1",
      OPENAI_API_KEY : data.coder_workspace_owner.me.session_token,
    } : {}
  )
  startup_script_behavior = "blocking"

  display_apps {
    vscode          = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode") && try(data.coder_parameter.vscode_channel[0].value, "stable") == "stable"
    vscode_insiders = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode") && try(data.coder_parameter.vscode_channel[0].value, "stable") == "insiders"
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
    script       = "coder stat disk --path /home/coder"
    interval     = 600
    timeout      = 10
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
  }

  startup_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    function cleanup() {
      coder exp sync complete agent-startup
      touch /tmp/.coder-startup-script.done
    }
    trap cleanup EXIT
    coder exp sync start agent-startup

    # Authenticate GitHub CLI.
    if ! gh api user --jq .login >/dev/null 2>&1; then
      echo "Logging into GitHub CLI…"
      if ! coder external-auth access-token github | gh auth login --hostname github.com --with-token; then
        echo "GitHub CLI authentication failed; gh commands may not work."
      fi
    else
      echo "GitHub CLI already has working credentials."
    fi

    # Configure Mux GitHub owner login for browser access.
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

    # Docker daemon is managed by envbox — wait for it to become
    # available rather than starting it ourselves.
    timeout 120 bash -c 'until docker info >/dev/null 2>&1; do sleep 2; done' || echo "Warning: Docker daemon not ready after 120s"

    # Increase the shutdown timeout of the inner docker daemon so
    # cleanup scripts have enough time to run.
    sudo sh -c 'jq ". += {\"shutdown-timeout\": 240}" /etc/docker/daemon.json > /tmp/daemon.json.new && mv /tmp/daemon.json.new /etc/docker/daemon.json'
    sudo service docker restart || true
  EOT

  shutdown_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Clean up the Go build cache.
    go clean -cache

    # Clean up the coder build directory.
    rm -rf "${local.repo_dir}/build"

    # Clean up unused Docker resources.
    docker system prune -a -f

    # Remove dangling named volumes older than 30 days.
    KEEP_DAYS=30
    docker volume ls -qf dangling=true \
      | xargs -r docker volume inspect \
      | jq -r --argjson days "$KEEP_DAYS" '.[] | select(.CreatedAt != null) | ((now - (.CreatedAt | fromdateiso8601)) / 86400 | floor) as $a | select($a >= $days) | "\($a)\t\(.Name)"' \
      | while IFS=$'\t' read -r age name; do
      echo "Removing volume $name ($age d)"
      docker volume rm "$name" >/dev/null
    done
  EOT
}

# ---------------------------------------------------------------------------
# Modules
# ---------------------------------------------------------------------------

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
  version  = "1.4.1"
  agent_id = coder_agent.dev.id
}

module "git-config" {
  count              = data.coder_workspace.me.start_count
  source             = "dev.registry.coder.com/coder/git-config/coder"
  version            = "1.0.33"
  agent_id           = coder_agent.dev.id
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
  count                = data.coder_workspace.me.start_count
  source               = "registry.coder.com/coder/mux/coder"
  version              = "1.4.3"
  agent_id             = coder_agent.dev.id
  subdomain            = true
  display_name         = "Mux"
  add_project          = local.repo_dir
  install_version      = "next"
  package_manager      = "bun"
  restart_on_kill      = true
  max_restart_attempts = 10
}

module "code-server" {
  count                   = contains(jsondecode(data.coder_parameter.ide_choices.value), "code-server") ? data.coder_workspace.me.start_count : 0
  source                  = "dev.registry.coder.com/coder/code-server/coder"
  version                 = "1.4.4"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  auto_install_extensions = true
  group                   = "Web Editors"
}

module "vscode-web" {
  count                   = contains(jsondecode(data.coder_parameter.ide_choices.value), "vscode-web") ? data.coder_workspace.me.start_count : 0
  source                  = "dev.registry.coder.com/coder/vscode-web/coder"
  version                 = "1.5.0"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  extensions              = ["github.copilot"]
  auto_install_extensions = true
  accept_license          = true
  group                   = "Web Editors"
}

module "jetbrains" {
  count         = contains(jsondecode(data.coder_parameter.ide_choices.value), "jetbrains") ? data.coder_workspace.me.start_count : 0
  source        = "dev.registry.coder.com/coder/jetbrains/coder"
  version       = "1.3.1"
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
  version  = "1.4.1"
  agent_id = coder_agent.dev.id
  folder   = local.repo_dir
}

module "windsurf" {
  count    = contains(jsondecode(data.coder_parameter.ide_choices.value), "windsurf") ? data.coder_workspace.me.start_count : 0
  source   = "dev.registry.coder.com/coder/windsurf/coder"
  version  = "1.3.1"
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

module "portabledesktop" {
  source   = "dev.registry.coder.com/coder/portabledesktop/coder"
  version  = "0.1.0"
  agent_id = coder_agent.dev.id
}

# ---------------------------------------------------------------------------
# Scripts
# ---------------------------------------------------------------------------

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

    # Install playwright dependencies.
    # We want to use the playwright version from site/package.json.
    cd "${local.repo_dir}" && make clean
    cd "${local.repo_dir}/site" && pnpm install
  EOT
}

resource "coder_script" "go-cache-cleanup-cron" {
  agent_id     = coder_agent.dev.id
  display_name = "Go Build Cache Cleanup Cron"
  icon         = "${data.coder_workspace.me.access_url}/emojis/1f9f9.png" // 🧹
  cron         = "0 ${local.cache_cleanup_minute} ${local.cache_cleanup_hour} * * *"
  script       = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail

    cache_dir=$(go env GOCACHE)
    echo "Cleaning Go build cache entries not used in the last 2 days..."
    before=$(du -s "$cache_dir" 2>/dev/null | awk '{print $1}')
    find "$cache_dir" -type f -mtime +2 -delete
    find "$cache_dir" -type d -empty -delete
    after=$(du -s "$cache_dir" 2>/dev/null | awk '{print $1}')
    freed=$(( (before - after) / 1024 ))
    echo "Freed $${freed}MB from Go build cache."
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
    # The boundary config is baked into the workspace image so
    # we create a stub here for the K8s template.
    if [ -f /etc/coder_boundary/config.yaml ]; then
      cp /etc/coder_boundary/config.yaml ~/.config/coder_boundary/config.yaml
    fi
    chmod 600 ~/.config/coder_boundary/config.yaml 2>/dev/null || true
  EOF
}

resource "coder_devcontainer" "coder" {
  count            = data.coder_parameter.devcontainer_autostart.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.dev.id
  workspace_folder = local.repo_dir
}

# ---------------------------------------------------------------------------
# AI Task Support
# ---------------------------------------------------------------------------

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

module "claude-code" {
  count               = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.9.1"
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

# ---------------------------------------------------------------------------
# Envbox Pod
# ---------------------------------------------------------------------------

resource "kubernetes_pod_v1" "workspace" {
  count = data.coder_workspace.me.start_count

  metadata {
    name      = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}"
    namespace = var.namespace
    labels    = local.workspace_labels
    annotations = {
      "com.coder.user.email" = data.coder_workspace_owner.me.email
    }
  }

  spec {
    restart_policy                   = "Never"
    termination_grace_period_seconds = 300
    automount_service_account_token  = false

    container {
      name              = "dev"
      image             = "ghcr.io/coder/envbox:latest"
      image_pull_policy = "Always"
      command           = ["/envbox", "docker"]

      security_context {
        privileged = true
      }

      resources {
        requests = {
          "cpu"    = "2"
          "memory" = "4Gi"
        }
        limits = {
          "cpu"    = "${data.coder_parameter.cpu.value}"
          "memory" = "${data.coder_parameter.memory.value}Gi"
        }
      }

      # -- Envbox configuration --

      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }

      env {
        name  = "CODER_AGENT_URL"
        value = data.coder_workspace.me.access_url
      }

      env {
        name  = "CODER_INNER_IMAGE"
        value = data.coder_parameter.image_type.value
      }

      env {
        name  = "CODER_INNER_USERNAME"
        value = "coder"
      }

      env {
        name  = "CODER_BOOTSTRAP_SCRIPT"
        value = coder_agent.dev.init_script
      }

      env {
        name  = "CODER_MOUNTS"
        value = "/home/coder:/home/coder"
      }

      env {
        name  = "CODER_INNER_HOSTNAME"
        value = data.coder_workspace.me.name
      }

      # Pass environment variables to the inner container. These
      # mirror the env list on the Docker dogfood template's
      # docker_container resource.
      env {
        name  = "CODER_INNER_ENVS"
        value = "CODER_AGENT_DEVCONTAINERS_ENABLE=1,CODER_PROC_PRIO_MGMT=1,CODER_PROC_OOM_SCORE=10,CODER_PROC_NICE_SCORE=1,USE_CAP_NET_ADMIN=true"
      }

      env {
        name = "CODER_CPUS"
        value_from {
          resource_field_ref {
            resource = "limits.cpu"
          }
        }
      }

      env {
        name = "CODER_MEMORY"
        value_from {
          resource_field_ref {
            resource = "limits.memory"
          }
        }
      }

      # -- Volume mounts --

      # Home PVC: persistent user data.
      volume_mount {
        name       = "home"
        mount_path = "/home/coder"
        sub_path   = "home"
      }

      # Docker PVC: envbox cache and Docker layers.
      volume_mount {
        name       = "docker"
        mount_path = "/var/lib/coder/docker"
        sub_path   = "cache/docker"
      }

      volume_mount {
        name       = "docker"
        mount_path = "/var/lib/coder/containers"
        sub_path   = "cache/containers"
      }

      volume_mount {
        name       = "docker"
        mount_path = "/var/lib/containers"
        sub_path   = "envbox/containers"
      }

      volume_mount {
        name       = "docker"
        mount_path = "/var/lib/docker"
        sub_path   = "envbox/docker"
      }

      # Sysbox scratch space (ephemeral).
      volume_mount {
        name       = "sysbox"
        mount_path = "/var/lib/sysbox"
      }

      # Kernel headers for sysbox inside envbox.
      volume_mount {
        name       = "usr-src"
        mount_path = "/usr/src"
      }

      volume_mount {
        name       = "lib-modules"
        mount_path = "/lib/modules"
      }
    }

    # -- Volumes --

    volume {
      name = "home"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.home.metadata[0].name
        read_only  = false
      }
    }

    volume {
      name = "docker"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.docker.metadata[0].name
        read_only  = false
      }
    }

    volume {
      name = "sysbox"
      empty_dir {}
    }

    volume {
      name = "usr-src"
      host_path {
        path = "/usr/src"
        type = ""
      }
    }

    volume {
      name = "lib-modules"
      host_path {
        path = "/lib/modules"
        type = ""
      }
    }

    affinity {
      pod_anti_affinity {
        preferred_during_scheduling_ignored_during_execution {
          weight = 1
          pod_affinity_term {
            topology_key = "kubernetes.io/hostname"
            label_selector {
              match_expressions {
                key      = "app.kubernetes.io/name"
                operator = "In"
                values   = ["coder-workspace"]
              }
            }
          }
        }
      }
    }
  }
}

resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = kubernetes_pod_v1.workspace[0].id
  item {
    key   = "cpu"
    value = "${data.coder_parameter.cpu.value} cores"
  }
  item {
    key   = "memory"
    value = "${data.coder_parameter.memory.value} GB"
  }
  item {
    key   = "home_disk"
    value = "${data.coder_parameter.home_disk_size.value} GB"
  }
  item {
    key   = "docker_disk"
    value = "${data.coder_parameter.docker_disk_size.value} GB"
  }
  item {
    key   = "image"
    value = data.coder_parameter.image_type.value
  }
  item {
    key   = "ai_task"
    value = data.coder_task.me.enabled ? "yes" : "no"
  }
}
