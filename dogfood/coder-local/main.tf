terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 4.0"
    }
  }
}

locals {
  repo_base_dir  = data.coder_parameter.repo_base_dir.value == "~" ? "/home/coder" : replace(data.coder_parameter.repo_base_dir.value, "/^~\\//", "/home/coder/")
  repo_dir       = replace(try(module.git-clone[0].repo_dir, ""), "/^~\\//", "/home/coder/")
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

  // Derive a stable per-workspace hour and minute from the workspace ID
  // so that cache cleanup crons don't all hit the filesystem at once.
  cache_cleanup_hour   = parseint(substr(data.coder_workspace.me.id, 0, 2), 16) % 24
  cache_cleanup_minute = parseint(substr(data.coder_workspace.me.id, 2, 2), 16) % 60
}

data "coder_parameter" "repo_base_dir" {
  type        = "string"
  name        = "Coder Repository Base Directory"
  default     = "~"
  description = "The directory specified will be created (if missing) and [coder/coder](https://github.com/coder/coder) will be automatically cloned into [base directory]/coder."
  mutable     = true
}

locals {
  image = "codercom/oss-dogfood:26.04"
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

data "coder_parameter" "enable_ai_gateway" {
  type        = "bool"
  name        = "Use AI Gateway"
  default     = true
  description = "If enabled, AI requests will be sent via AI Gateway."
  mutable     = true
}

# Only used if AI Gateway is disabled.
variable "anthropic_api_key" {
  type        = string
  description = "The API key used to authenticate with the Anthropic API, if AI Gateway is disabled."
  default     = ""
  sensitive   = true
}

variable "openai_api_key" {
  type        = string
  description = "The API key used to authenticate with the OpenAI API, if AI Gateway is disabled."
  default     = ""
  sensitive   = true
}

// Docker provider inherits DOCKER_HOST from the environment.
// No host is set here so the user-local provisioner uses the
// socket configured on the developer's machine.
provider "docker" {}
provider "coder" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_workspace_tags" "tags" {
  tags = {
    "scope" = "user"
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
  version  = "1.4.1"
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

module "git-clone" {
  count             = data.coder_workspace.me.start_count
  source            = "dev.registry.coder.com/coder/git-clone/coder"
  version           = "1.3.0"
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
  auto_install_extensions = true # will install extensions from the repos .vscode/extensions.json file
  accept_license          = true
  group                   = "Web Editors"
}

module "jetbrains" {
  count         = contains(jsondecode(data.coder_parameter.ide_choices.value), "jetbrains") ? data.coder_workspace.me.start_count : 0
  source        = "dev.registry.coder.com/coder/jetbrains/coder"
  version       = "1.4.0"
  agent_id      = coder_agent.dev.id
  agent_name    = "dev"
  folder        = local.repo_dir
  major_version = "latest"
  tooltip       = "You need to [install JetBrains Toolbox](https://coder.com/docs/user-guides/workspace-access/jetbrains/toolbox) to use this app."
}

module "filebrowser" {
  count      = data.coder_workspace.me.start_count
  source     = "dev.registry.coder.com/coder/filebrowser/coder"
  version    = "1.1.5"
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

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = merge(
    {
      OIDC_TOKEN : data.coder_workspace_owner.me.oidc_access_token,
    },
    data.coder_parameter.enable_ai_gateway.value ? {
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

  # Warn when the Docker VM (or host) has less than 16Gi of memory.
  # On macOS with Docker Desktop, /proc/meminfo reflects the Linux VM
  # memory allocation configured in Docker Desktop settings.
  metadata {
    display_name = "Host Memory"
    key          = "host_memory"
    order        = 3
    script       = <<-EOT
      #!/bin/bash
      total_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
      total_gb=$((total_kb / 1024 / 1024))
      if [ "$total_gb" -lt 16 ]; then
        echo "$${total_gb}Gi (WARNING: <16Gi)"
      else
        echo "$${total_gb}Gi"
      fi
    EOT
    interval     = 600
    timeout      = 5
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
    volume {
      enabled   = true
      threshold = data.coder_parameter.res_mon_volume_threshold.value
      path      = data.coder_parameter.res_mon_volume_path.value
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

    # Authenticate GitHub CLI. `gh api user` is used instead of `gh auth
    # status` because the latter exits non-zero when a stale token exists
    # in ~/.config/gh/hosts.yml, even when a valid GITHUB_TOKEN is already
    # present in the environment and gh commands work fine.
    if ! gh api user --jq .login >/dev/null 2>&1; then
      echo "Logging into GitHub CLI..."
      if ! coder external-auth access-token github | gh auth login --hostname github.com --with-token; then
        echo "GitHub CLI authentication failed; gh commands may not work."
      fi
    else
      echo "GitHub CLI already has working credentials."
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

  shutdown_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Clean up the Go build cache to prevent the home volume from
    # accumulating waste and growing too large.
    go clean -cache

    # Clean up the coder build directory as this can get quite large.
    rm -rf "${local.repo_dir}/build"
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

resource "coder_metadata" "home_volume" {
  resource_id = docker_volume.home_volume.id
  daily_cost  = 0
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

resource "coder_metadata" "homebrew_volume" {
  resource_id = docker_volume.homebrew_volume.id
  hide        = true # Hide it as it only backs Homebrew state.
}

resource "docker_volume" "homebrew_volume" {
  name = "coder-${data.coder_workspace.me.id}-homebrew"
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
  name = local.image
}

resource "docker_image" "dogfood" {
  name          = "${local.image}@${data.docker_registry_image.dogfood.sha256_digest}"
  pull_triggers = [data.docker_registry_image.dogfood.sha256_digest]
  keep_locally  = true
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = docker_image.dogfood.name
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  # No memory limit; no sysbox-runc runtime.
  restart = "unless-stopped"

  # Shorter grace period than the remote template since there is no inner
  # Docker daemon to wait for.
  destroy_grace_seconds = 60
  stop_timeout          = 60
  stop_signal           = "SIGINT"

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
    "CODER_AGENT_EXP_MCP_CONFIG_FILES=~/.mcp.json,.mcp.json",
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
  # Homebrew is baked into this path. A Docker named volume copies the
  # image contents on first mount, then persists user-installed formulae.
  volumes {
    container_path = "/home/linuxbrew/"
    volume_name    = docker_volume.homebrew_volume.name
    read_only      = false
  }
  # Mount the host Docker socket for Docker-outside-of-Docker (DooD).
  # The workspace process can run Docker commands against the host daemon.
  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
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
    key   = "image"
    value = local.image
  }
}

module "claude-code" {
  count             = data.coder_workspace.me.start_count
  source            = "dev.registry.coder.com/coder/claude-code/coder"
  version           = "5.2.0"
  enable_ai_gateway = data.coder_parameter.enable_ai_gateway.value
  anthropic_api_key = data.coder_parameter.enable_ai_gateway.value ? "" : var.anthropic_api_key
  agent_id          = coder_agent.dev.id
  workdir           = local.repo_dir
  mcp               = <<-EOF
    {
      "mcpServers": {
        "playwright": {
          "command": "npx",
          "args": ["--", "@playwright/mcp@latest", "--headless", "--isolated", "--no-sandbox"]
        }
      }
    }
  EOF
}

resource "coder_app" "claude" {
  agent_id     = coder_agent.dev.id
  slug         = "claude"
  display_name = "Claude Code"
  icon         = "/icon/claude.svg"
  open_in      = "slim-window"
  command      = <<-EOT
    #!/bin/bash
    set -e
    cd "${local.repo_dir}"
    exec tmux new-session -A -s claude claude
  EOT
}

module "codex" {
  source            = "dev.registry.coder.com/coder-labs/codex/coder"
  version           = "5.0.0"
  agent_id          = coder_agent.dev.id
  workdir           = local.repo_dir
  enable_ai_gateway = data.coder_parameter.enable_ai_gateway.value
  openai_api_key    = data.coder_parameter.enable_ai_gateway.value ? "" : var.openai_api_key
  mcp               = <<-EOT
    [mcp_servers.playwright]
    command = "npx"
    args = ["--", "@playwright/mcp@latest", "--headless", "--isolated", "--no-sandbox"]
    type = "stdio"
  EOT
}

resource "coder_app" "codex" {
  agent_id     = coder_agent.dev.id
  slug         = "codex"
  display_name = "Codex"
  icon         = "/icon/openai-codex.svg"
  open_in      = "slim-window"
  command      = <<-EOT
    #!/bin/bash
    set -e
    cd "${local.repo_dir}"
    exec tmux new-session -A -s codex codex
  EOT
}
