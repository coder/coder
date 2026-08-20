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

variable "label" {
  type = string
}

resource "coder_script" "nested" {
  agent_id     = var.agent_id
  display_name = "${var.label} nested"
  script       = "echo ${var.label} nested"
  run_on_start = true
}
