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

provider "docker" {}
provider "coder" {}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  username = data.coder_workspace_owner.me.name
}

# Parameter for users to enter prompts directly from the workspace creation page
data "coder_parameter" "ai_prompt" {
  type        = "string"
  name        = "AI Prompt"
  default     = ""
  description = "Write a prompt for Claude Code"
  mutable     = true
  ephemeral   = true
}

resource "coder_agent" "main" {
  arch = "amd64"
  os   = "linux"
  dir  = "/home/${local.username}"
  
  startup_script = <<-EOT
    # Ensure screen is installed (required for Claude Code)
    if ! command -v screen &> /dev/null; then
      echo "Installing screen for Claude Code..."
      sudo apt-get update
      sudo apt-get install -y screen
    fi
    
    # Ensure Node.js and npm are installed
    if ! command -v node &> /dev/null; then
      echo "Installing Node.js and npm..."
      curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
      sudo apt-get install -y nodejs
    fi
  EOT

  env = {
    # Git configuration
    GIT_AUTHOR_NAME     = data.coder_workspace_owner.me.name
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = data.coder_workspace_owner.me.name
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }

  # Basic workspace metrics
  metadata {
    display_name = "CPU Usage"
    key          = "cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }
}

# Integrate Claude Code using the official module
module "claude-code" {
  source  = "registry.coder.com/modules/claude-code/coder"
  version = "1.2.1"
  agent_id = coder_agent.main.id
  
  # Enable dashboard visualization features
  experiment_use_screen = true
  experiment_report_tasks = true
}

# Configure Claude with Anthropic API key and Sonnet model
resource "coder_env" "claude_api_key" {
  agent_id = coder_agent.main.id
  name     = "CLAUDE_API_KEY"
  value    = var.anthropic_api_key
}

resource "coder_env" "claude_model" {
  agent_id = coder_agent.main.id
  name     = "CLAUDE_MODEL" 
  value    = "claude-3-sonnet-20240229"
}

# Pass parameter prompt to Claude Code
resource "coder_env" "claude_task_prompt" {
  agent_id = coder_agent.main.id
  name     = "CODER_MCP_CLAUDE_TASK_PROMPT"
  value    = data.coder_parameter.ai_prompt.value
}

# Add VS Code integration
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.2"
  agent_id = coder_agent.main.id
  folder   = "/home/${local.username}"
}

# Docker resources
resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
}

resource "docker_image" "main" {
  name = "codercom/enterprise-base:ubuntu"
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = docker_image.main.name
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/home/${local.username}"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  
  labels {
    label = "coder.workspace_id" 
    value = data.coder_workspace.me.id
  }
}

# Parameter for API authentication
variable "anthropic_api_key" {
  type        = string
  sensitive   = true
  description = "Anthropic API key for Claude Sonnet integration"
}

# Output with usage instructions
output "claude_usage_instructions" {
  value = "Claude 3 Sonnet is available in this workspace. Use it through the terminal with 'claude-code \"Your prompt\"' or through the VS Code extension."
}