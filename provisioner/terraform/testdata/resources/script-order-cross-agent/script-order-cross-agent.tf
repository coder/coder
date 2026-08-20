terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "one" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "two" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_script" "one" {
  agent_id     = coder_agent.one.id
  display_name = "Agent one"
  script       = "echo one"
  run_on_start = true
}

resource "coder_script" "two" {
  agent_id     = coder_agent.two.id
  display_name = "Agent two"
  script       = "echo two"
  run_on_start = true
}

data "coder_script_order" "cross_agent" {
  rule {
    run   = ["coder_script.two"]
    after = ["coder_script.one"]
  }
}

resource "null_resource" "one" {
  depends_on = [coder_agent.one]
}

resource "null_resource" "two" {
  depends_on = [coder_agent.two]
}
