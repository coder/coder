terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_env" "path_b" {
  agent_id = coder_agent.dev.id
  name     = "PATH"
  value    = "/b/bin"
}

resource "coder_env" "path_a" {
  agent_id = coder_agent.dev.id
  name     = "PATH"
  value    = "/a/bin"
}

resource "coder_env" "unique_env" {
  agent_id = coder_agent.dev.id
  name     = "UNIQUE"
  value    = "unique_value"
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev
  ]
}
