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

resource "coder_env" "env1" {
  agent_id = coder_agent.dev1.id
  name     = "ENV_1"
  value    = "Env 1"
}

resource "coder_env" "env2" {
  agent_id = coder_agent.dev1.id
  name     = "ENV_2"
  value    = "Env 2"
}

resource "coder_env" "env3" {
  agent_id = coder_agent.dev2.id
  name     = "ENV_3"
  value    = "Env 3"
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
