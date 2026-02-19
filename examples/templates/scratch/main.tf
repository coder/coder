terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_provisioner" "me" {}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = data.coder_provisioner.me.os

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
}

# Use this to set environment variables in your workspace
# details: https://registry.terraform.io/providers/coder/coder/latest/docs/resources/env
resource "coder_env" "welcome_message" {
  agent_id   = coder_agent.main.id
  agent_name = "main"
  name       = "WELCOME_MESSAGE"
  value      = "Welcome to your Coder workspace!"
}

# Adds code-server
# See all available modules at https://registry.coder.com/modules
module "code-server" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/coder/code-server/coder"

  # This ensures that the latest non-breaking version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = "~> 1.0"

  agent_id   = coder_agent.main.id
  agent_name = "main"
}

# Runs a script at workspace start/stop or on a cron schedule
# details: https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script
resource "coder_script" "startup_script" {
  agent_id           = coder_agent.main.id
  agent_name         = "main"
  display_name       = "Startup Script"
  script             = <<-EOF
    #!/bin/sh
    set -e
    # Run programs at workspace startup
  EOF
  run_on_start       = true
  start_blocks_login = true
}
