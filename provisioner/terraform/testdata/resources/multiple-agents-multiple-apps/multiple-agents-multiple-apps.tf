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

# app1 is for testing subdomain default.
resource "coder_app" "app1" {
  agent_id = coder_agent.dev1.id
  slug     = "app1"
  # subdomain should default to false.
  # subdomain = false
}

# app2 tests that subdomaincan be true, and that healthchecks work.
resource "coder_app" "app2" {
  agent_id  = coder_agent.dev1.id
  slug      = "app2"
  subdomain = true
  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

# app3 tests that subdomain can explicitly be false.
resource "coder_app" "app3" {
  agent_id  = coder_agent.dev2.id
  slug      = "app3"
  subdomain = false
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
