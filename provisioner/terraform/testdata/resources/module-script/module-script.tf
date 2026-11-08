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

# One script in the root module and one in a child module, so the plan
# and state goldens show how each source handles a module script.
resource "coder_script" "root" {
  agent_id     = coder_agent.main.id
  display_name = "Root Script"
  script       = "echo root"

  run_on_start = true
}

module "module" {
  source   = "./module"
  agent_id = coder_agent.main.id
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}
