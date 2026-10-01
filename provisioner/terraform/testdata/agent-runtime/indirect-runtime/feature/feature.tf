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

variable "name" {
  type = string
}

resource "coder_script" "work" {
  agent_id     = var.agent_id
  display_name = "Feature ${var.name}"
  script       = "echo feature-${var.name}"
  run_on_start = true
}
