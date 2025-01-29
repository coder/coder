terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
  }
}

resource "coder_agent" "dev1" {
  os   = "linux"
  arch = "amd64"
  resources_monitoring {
    memory {
      enabled   = true
      threshold = 80
    }
  }
}

resource "coder_agent" "dev2" {
  os   = "linux"
  arch = "amd64"
  resources_monitoring {
    memory {
      enabled   = true
      threshold = 99
    }
    volume {
      path      = "volume1"
      enabled   = true
      threshold = 80
    }
    volume {
      path      = "volume2"
      enabled   = false
      threshold = 50
    }
  }
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

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1,
    coder_agent.dev2
  ]
}
