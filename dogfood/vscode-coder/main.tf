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

  repo_base_dir  = data.coder_parameter.repo_base_dir.value == "~" ? "/home/coder" : replace(data.coder_parameter.repo_base_dir.value, "/^~\\//", "/home/coder/")
  repo_dir       = replace(try(module.git-clone[0].repo_dir, ""), "/^~\\//", "/home/coder/")
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

# --- Parameters ---

data "coder_parameter" "repo_base_dir" {
  type        = "string"
  name        = "Repository Base Directory"
  default     = "~"
  description = "The directory specified will be created (if missing) and [coder/vscode-coder](https://github.com/coder/vscode-coder) will be automatically cloned into [base directory]/vscode-coder."
  mutable     = true
}

locals {
  default_regions = {
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

data "coder_parameter" "use_ai_bridge" {
  type        = "bool"
  name        = "Use AI Bridge"
  default     = true
  description = "If enabled, AI requests will be sent via AI Bridge."
  mutable     = true
}

# Fallback when AI Bridge is disabled. Injected by dogfood/main.tf
# from the CODER_DOGFOOD_ANTHROPIC_API_KEY secret.
variable "anthropic_api_key" {
  type        = string
  description = "Anthropic API key, used when AI Bridge is disabled."
  default     = ""
  sensitive   = true
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

# --- Providers and data sources ---

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

# --- Modules ---

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
  url               = "https://github.com/coder/vscode-coder"
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

# --- Agent ---

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
    script       = "sudo du -sh /home/coder | awk '{print $1}'"
    interval     = 3600
    timeout      = 60
  }

  metadata {
    display_name = "Word of the Day"
    key          = "word"
    order        = 3
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

    # Start dbus to suppress noisy Electron/Chromium errors in tests.
    sudo mkdir -p /run/dbus
    sudo dbus-daemon --system 2>/dev/null || true

    if ! gh api user --jq .login >/dev/null 2>&1; then
      echo "Logging into GitHub CLI..."
      if ! coder external-auth access-token github | gh auth login --hostname github.com --with-token; then
        echo "GitHub CLI authentication failed; gh commands may not work."
      fi
    else
      echo "GitHub CLI already has working credentials."
    fi
  EOT
}

# --- Scripts ---

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

    cd "${local.repo_dir}" && pnpm install

    # Playwright maintains the list of system libs each Chromium version
    # needs, so we don't have to track them in the Dockerfile.
    # Electron additionally requires libgtk-3-0 which Playwright skips
    # because its bundled Chromium doesn't use GTK. xauth is needed
    # by xvfb-run for integration tests.
    sudo npx --yes playwright install-deps chromium
    sudo apt-get install -y --no-install-recommends libgtk-3-0 xauth
  EOT
}

# --- Docker resources ---

resource "coder_metadata" "home_volume" {
  resource_id = docker_volume.home_volume.id
  daily_cost  = 1
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
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
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

data "docker_registry_image" "vscode_coder" {
  name = "codercom/oss-dogfood-vscode-coder:latest"
}

resource "docker_image" "vscode_coder" {
  name = "codercom/oss-dogfood-vscode-coder:latest@${data.docker_registry_image.vscode_coder.sha256_digest}"
  pull_triggers = [
    data.docker_registry_image.vscode_coder.sha256_digest,
    filesha1("Dockerfile"),
  ]
  keep_locally = true
}

resource "docker_container" "workspace" {
  lifecycle {
    ignore_changes = [name, hostname, labels, env, entrypoint]
  }
  count      = data.coder_workspace.me.start_count
  image      = docker_image.vscode_coder.name
  name       = local.container_name
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  memory     = 8192

  # Allow the agent to finish cleanup traps on shutdown.
  destroy_grace_seconds = 60
  stop_timeout          = 60
  stop_signal           = "SIGINT"

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
  item {
    key   = "ai_task"
    value = data.coder_task.me.enabled ? "yes" : "no"
  }
}

# --- AI task support ---

locals {
  claude_system_prompt = <<-EOT
    -- Framing --
    You are a helpful coding assistant working on the coder/vscode-coder
    VS Code extension. Aim to autonomously investigate and solve issues
    the user gives you and test your work, whenever possible.

    Avoid shortcuts like mocking tests. When you get stuck, you can ask
    the user but opt for autonomy.

    -- Tool Selection --
    - Built-in tools for everything:
      (file operations, git commands, builds & installs, one-off shell commands)

    -- Testing --
    Integration tests launch a real VS Code instance and require a
    virtual framebuffer. Run them headlessly with:
      xvfb-run -a pnpm test:integration
    This matches how CI runs them. Unit tests do not need xvfb-run:
      pnpm test

    -- Workflow --
    When starting new work:
    1. If given a GitHub issue URL, use the `gh` CLI to read the full
       issue details with `gh issue view <issue-number>`.
    2. Create a feature branch for the work using a descriptive name
       based on the issue or task.
       Example: `git checkout -b fix/issue-123-ssh-retry`
    3. Proceed with implementation following the AGENTS.md guidelines.

    -- Context --
    This is the coder/vscode-coder VS Code extension. It is a real-world
    production extension used by developers to connect to Coder workspaces.
    Be sure to read AGENTS.md before making any changes.
  EOT
}

module "claude-code" {
  count               = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.9.2"
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
}

resource "coder_ai_task" "task" {
  count  = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  app_id = module.claude-code[count.index].task_app_id
}

resource "coder_app" "watch" {
  count        = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  agent_id     = coder_agent.dev.id
  slug         = "watch"
  display_name = "pnpm watch"
  icon         = "${data.coder_workspace.me.access_url}/icon/code.svg"
  command      = "screen -x pnpm_watch"
  share        = "authenticated"
  open_in      = "tab"
  order        = 0
}

resource "coder_script" "watch" {
  count              = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
  display_name       = "pnpm watch"
  agent_id           = coder_agent.dev.id
  run_on_start       = true
  start_blocks_login = false
  icon               = "${data.coder_workspace.me.access_url}/icon/code.svg"
  script             = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    trap 'coder exp sync complete pnpm-watch' EXIT
    coder exp sync want pnpm-watch install-deps
    coder exp sync start pnpm-watch

    cd "${local.repo_dir}" && screen -dmS pnpm_watch /bin/sh -c 'while true; do pnpm watch; echo "pnpm watch exited with code $? restarting in 10s"; sleep 10; done'
  EOT
}
