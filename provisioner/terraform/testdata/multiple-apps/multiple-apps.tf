terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.15"
    }
  }
}

resource "coder_agent" "dev1" {
  os   = "linux"
  arch = "amd64"
}

# app1 is for testing relative_path default.
resource "coder_app" "app1" {
  agent_id = coder_agent.dev1.id
  # relative_path should default to true.
  # relative_path = true
}

# app2 tests that relative_path can be false, and that healthchecks work.
resource "coder_app" "app2" {
  agent_id      = coder_agent.dev1.id
  relative_path = false
  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

# app3 tests that relative_path can explicitly be true.
resource "coder_app" "app3" {
  agent_id      = coder_agent.dev1.id
  relative_path = true
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1
  ]
}
