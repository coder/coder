terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

module "module" {
  source = "./module"
  script = coder_agent.main.init_script
}
