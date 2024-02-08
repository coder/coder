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
  arch                   = data.coder_provisioner.me.arch
  os                     = data.coder_provisioner.me.os
  startup_script_timeout = 180
  startup_script         = <<-EOT
    set -e
    # Run programs at workspace startup
  EOT

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
resource "coder_env" "my_env" {
  agent_id = coder_agent.main.id
  name     = "FOO"
  value    = "bar"
}

# Adds code-server
# See all available modules at https://regsitry.coder.com
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.2"
  agent_id = coder_agent.main.id
}

# Runs a script at workspace start/stop or on a cron schedule
# details: https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script
resource "coder_script" "my_script" {
  agent_id     = coder_agent.main.id
  display_name = "My Script"
  run_on_start = true
  script       = <<-EOF
  echo "Hello ${data.coder_workspace.me.owner}!"
  EOF
}
