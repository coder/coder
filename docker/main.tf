terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.7.0"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

data "coder_parameter" "ai_prompt" {
  type        = "string"
  name        = "AI Prompt"
  default     = ""
  description = "Write a prompt for Claude Code"
  mutable     = true
}

module "claude-code" {
  source              = "registry.coder.com/modules/claude-code/coder"
  agent_id            = coder_agent.main.id
  folder              = "/home/coder"
  install_claude_code = true
  claude_code_version = "0.2.74"
  agentapi_version    = "v0.2.3"

  # experiment_pre_install_script = <<-EOT
  # gh auth setup-git
  # EOT

  # experiment_post_install_script = <<-EOT
  # npm i -g @executeautomation/playwright-mcp-server@1.0.1 @wonderwhy-er/desktop-commander@0.1.19

  # claude mcp add playwright playwright-mcp-server
  # claude mcp add desktop-commander desktop-commander

  # cd $(dirname $(which playwright-mcp-server))
  # cd $(dirname $(readlink playwright-mcp-server))
  # sudo sed -i 's/headless = false/headless = true/g' toolHandler.js
  # EOT


  # Icon is not available in Coder v2.20 and below, so we'll use a custom icon URL
  icon = "https://uxwing.com/wp-content/themes/uxwing/download/brands-and-social-media/claude-ai-icon.png"
  # Enable experimental features
  # experiment_use_screen          = true
  experiment_report_tasks = true
}


locals {
  username = data.coder_workspace_owner.me.name
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  # Defaulting to null if the variable is an empty string lets us have an optional variable without having to set our own default
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e

    # Prepare user home with default files on first start.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    # Add any commands that should be executed at workspace startup (e.g install requirements, start a program, etc) here
  EOT

  # These environment variables allow you to make Git commits right away after creating a
  # workspace. Note that they take precedence over configuration defined in ~/.gitconfig!
  # You can remove this block if you'd prefer to configure Git manually or using
  # dotfiles. (see docs/dotfiles.md)
  env = {
    GIT_AUTHOR_NAME                = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL               = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME             = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL            = "${data.coder_workspace_owner.me.email}"
    CLAUDE_CODE_USE_BEDROCK        = "1"
    AWS_REGION                     = "us-east-2"
    AWS_ACCESS_KEY_ID              = "MARCIN_KEY_ID"
    AWS_SECRET_ACCESS_KEY          = "MARCIN_SECRET"
    CODER_MCP_APP_STATUS_SLUG      = "claude-code"
    CODER_MCP_CLAUDE_TASK_PROMPT   = data.coder_parameter.ai_prompt.value
    CODER_MCP_CLAUDE_SYSTEM_PROMPT = <<-EOT
    Hey! The user will provide you with a prompt of something to create. Create it the best you can. 
    
    If web app:
      - ALWAYS use port 3000 so the user has a consistent preview to see their work
        - If the dev server is already running, kill the dev server to run on port 3000.
        - Avoid building the project for production. Just use dev servers (and ALWAYS VIA desktop-commander as mentioned below)
      - When you think you have finished, you should use Playwright to review the HTML to ensure it is working as expected.
        - Feel free to fix anything bad you see.

    Always run long-running commands (e.g. `pnpm dev` or `npm run dev`) using desktop-commander so it runs it in the background and users can prompt you.  Other short-lived commands (build, test, cd, write, read, view, etc) can run normally. 

    Never run the dev server without desktop-commander. This will cause you to stall and get stuck.

    For previewing, always use the dev server for fast feedback loops (never do a full Next.js build, for exmaple). Next.js or simple HTML/static 
    is preferred for web applications, but pick the best framework for the job.
    
    The dev server will be on localhost:3000 and NEVER start on another port. The user depends on localhost:3000. If the dev
    server crashes for some reason, kill port 3000 (or the desktop-commander session) and restart it.
    
    After large changes, use Playwright to ensure your changes work (preview localhost:3000). Take a screenshot, look at the screenshot. Also look at the HTML output from Playwright. If there are errors or something looks "off," fix it.

    Whenever waiting for a PR review, keep on retrying indefinitely until you get a review. Even if requests are timing out.

    Aim to autonomously investigate and solve issues the user gives you
    and test your work, whenever possible.

    Avoid shortcuts like mocking tests. When you get stuck, you can ask the user
    but opt for autonomy.
    
    Report every single task to Coder so that we can help you and understand where you are at
    following these EXACT guidelines:

    1. Be granular. If you are doing multiple steps, report each step
    to coder.
    2. IMMEDIATELY report status after receiving ANY user message
    3. Use "state": "working" when actively processing WITHOUT needing
    additional user input
    4. Use "state": "complete" only when finished with a task
    5. Use "state": "failure" when you need ANY user input, lack sufficient
    details, or encounter blockers


    In your summary:
    - Be specific about what you're doing
    - Clearly indicate what information you need from the user when in
    "failure" state
    - Keep it under 160 characters
    - Make it actionable

    When reporting URLs to Coder, do not use localhost. Instead, run `env | grep CODER) | and a URL like https://preview--dev--CODER_WORKSPACE_NAME--CODER_WORKSPACE_OWNER--apps.dev.coder.com/ but with it replaces with the proper env vars. That proxies port 3000.
    EOT
  }

  # The following metadata blocks are optional. They are used to display
  # information about your workspace in the dashboard. You can remove them
  # if you don't want to display any information.
  # For basic resources, you can use the `coder stat` command.
  # If you need more control, you can write your own script.
  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }

  metadata {
    display_name = "CPU Usage (Host)"
    key          = "4_cpu_usage_host"
    script       = "coder stat cpu --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Memory Usage (Host)"
    key          = "5_mem_usage_host"
    script       = "coder stat mem --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Load Average (Host)"
    key          = "6_load_host"
    # get load avg scaled by number of cores
    script   = <<EOT
      echo "`cat /proc/loadavg | awk '{ print $1 }'` `nproc`" | awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }

  metadata {
    display_name = "Swap Usage (Host)"
    key          = "7_swap_host"
    script       = <<EOT
      free -b | awk '/^Swap/ { printf("%.1f/%.1f", $3/1024.0/1024.0/1024.0, $2/1024.0/1024.0/1024.0) }'
    EOT
    interval     = 10
    timeout      = 1
  }
}

# See https://registry.coder.com/modules/code-server
module "code-server" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/code-server/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id = coder_agent.main.id
  order    = 1
}

# See https://registry.coder.com/modules/jetbrains-gateway
module "jetbrains_gateway" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/jetbrains-gateway/coder"

  # JetBrains IDEs to make available for the user to select
  jetbrains_ides = ["IU", "PS", "WS", "PY", "CL", "GO", "RM", "RD", "RR"]
  default        = "IU"

  # Default folder to open when starting a JetBrains IDE
  folder = "/home/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id   = coder_agent.main.id
  agent_name = "main"
  order      = 2
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

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
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
    container_path = "/home/coder"
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

