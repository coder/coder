terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

variable "agent_id" {
  type = string
}

resource "coder_script" "configure" {
  agent_id     = var.agent_id
  display_name = "Configure"
  script       = "echo configure"
  run_on_start = true
}
