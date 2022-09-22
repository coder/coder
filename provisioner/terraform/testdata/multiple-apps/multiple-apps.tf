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

resource "coder_app" "app1" {
  agent_id = coder_agent.dev1.id
}

resource "coder_app" "app2" {
  agent_id = coder_agent.dev1.id
  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1
  ]
}
