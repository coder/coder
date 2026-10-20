terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.20.0"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "terraform_data" "workspace" {
  input = coder_agent.main.id
}

module "bootstrap" {
  source   = "./bootstrap"
  agent_id = coder_agent.main.id
}

module "empty" {
  count = 0

  source   = "./bootstrap"
  agent_id = coder_agent.main.id
}

resource "coder_script" "work" {
  for_each = toset(["worker", "api"])

  agent_id     = coder_agent.main.id
  display_name = each.key
  script       = "echo ${each.key}"
  run_on_start = true
}

resource "coder_script" "optional" {
  count = 0

  agent_id     = coder_agent.main.id
  display_name = "Optional"
  script       = "echo optional"
  run_on_start = true
}

resource "coder_script" "unordered" {
  agent_id     = coder_agent.main.id
  display_name = "Unordered"
  script       = "echo unordered"
  run_on_start = true
}

resource "coder_script" "shutdown" {
  agent_id     = coder_agent.main.id
  display_name = "Shutdown"
  script       = "echo shutdown"
  run_on_stop  = true
}

data "coder_script_order" "main" {
  rule {
    run      = ["coder_script.work"]
    after    = ["module.bootstrap"]
    requires = "completion"
  }

  rule {
    run   = ["module.bootstrap"]
    after = ["coder_script.shutdown"]
    phase = "stop"
  }

  rule {
    run   = ["coder_script.optional"]
    after = ["module.empty"]
    phase = "start"
  }
}

data "coder_script_order" "overlapping" {
  rule {
    run      = ["coder_script.work[\"api\"]"]
    after    = ["module.bootstrap"]
    requires = "completion"
    phase    = "start"
  }
}
