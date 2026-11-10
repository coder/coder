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

resource "coder_script" "start" {
  count = 2

  agent_id     = var.agent_id
  display_name = "Start ${count.index}"
  script       = "echo start ${count.index}"
  run_on_start = true
}

resource "coder_script" "stop" {
  agent_id     = var.agent_id
  display_name = "Stop"
  script       = "echo stop"
  run_on_stop  = true
}

module "nested" {
  source   = "./nested"
  agent_id = var.agent_id
}

data "coder_script_order" "nested" {
  rule {
    run   = ["module.nested"]
    after = ["coder_script.start"]
    phase = "start"
  }
}
