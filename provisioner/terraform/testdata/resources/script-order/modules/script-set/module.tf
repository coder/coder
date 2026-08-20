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

resource "coder_script" "module_script" {
  agent_id     = var.agent_id
  display_name = "${var.label} module"
  script       = "echo ${var.label} module"
  run_on_start = true
}

module "nested" {
  source   = "./nested"
  agent_id = var.agent_id
  label    = var.label
}

data "coder_script_order" "nested" {
  rule {
    run   = ["module.nested"]
    after = ["coder_script.module_script"]
  }
}
