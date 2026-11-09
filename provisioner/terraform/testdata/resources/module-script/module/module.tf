terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

variable "agent_id" {
  type = string
}

resource "coder_script" "clone" {
  agent_id     = var.agent_id
  display_name = "Module Script"
  script       = "echo module"

  run_on_start = true
}
