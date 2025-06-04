terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "dev1" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "dev2" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_script" "script1" {
  agent_id     = coder_agent.dev1.id
  display_name = "Foobar Script 1"
  script       = "echo foobar 1"

  run_on_start = true
}

resource "coder_script" "script2" {
  agent_id     = coder_agent.dev1.id
  display_name = "Foobar Script 2"
  script       = "echo foobar 2"

  run_on_start = true
}

resource "coder_script" "script3" {
  agent_id     = coder_agent.dev2.id
  display_name = "Foobar Script 3"
  script       = "echo foobar 3"

  run_on_start = true
}

resource "null_resource" "dev1" {
  depends_on = [
    coder_agent.dev1
  ]
}

resource "null_resource" "dev2" {
  depends_on = [
    coder_agent.dev2
  ]
}
