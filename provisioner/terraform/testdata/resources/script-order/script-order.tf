terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_script" "root_start" {
  agent_id     = coder_agent.main.id
  display_name = "Root start"
  script       = "echo root start"
  run_on_start = true
}

resource "coder_script" "counted" {
  count = 2

  agent_id     = coder_agent.main.id
  display_name = "Counted ${count.index}"
  script       = "echo counted ${count.index}"
  run_on_start = true
}

resource "coder_script" "keyed" {
  for_each = toset(["api", "worker"])

  agent_id     = coder_agent.main.id
  display_name = "Keyed ${each.key}"
  script       = "echo keyed ${each.key}"
  run_on_start = true
}

resource "coder_script" "unordered" {
  agent_id     = coder_agent.main.id
  display_name = "Unordered"
  script       = "echo unordered"
  run_on_start = true
}

resource "coder_script" "stop_a" {
  agent_id     = coder_agent.main.id
  display_name = "Stop A"
  script       = "echo stop A"
  run_on_stop  = true
}

resource "coder_script" "stop_b" {
  agent_id     = coder_agent.main.id
  display_name = "Stop B"
  script       = "echo stop B"
  run_on_stop  = true
}

module "prerequisites" {
  source   = "./modules/script-set"
  agent_id = coder_agent.main.id
  label    = "prerequisite"
}

module "dependents" {
  source   = "./modules/script-set"
  agent_id = coder_agent.main.id
  label    = "dependent"
}

data "coder_script_order" "primary" {
  rule {
    run   = ["coder_script.counted"]
    after = ["coder_script.root_start"]
  }

  rule {
    run      = ["coder_script.keyed"]
    after    = ["coder_script.counted[1]"]
    requires = "completion"
  }

  rule {
    run      = ["module.dependents"]
    after    = ["module.prerequisites"]
    requires = "completion"
  }

  rule {
    run   = ["coder_script.stop_b"]
    after = ["coder_script.stop_a"]
  }
}

data "coder_script_order" "overlapping" {
  rule {
    run = [
      "coder_script.counted[1]",
      "coder_script.keyed[\"worker\"]",
    ]
    after = ["coder_script.root_start"]
  }

  rule {
    run      = ["module.dependents"]
    after    = ["module.prerequisites"]
    requires = "completion"
  }
}

resource "null_resource" "workspace" {
  depends_on = [coder_agent.main]
}
