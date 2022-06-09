terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
  }
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

module "module" {
  source = "./module"
  script = coder_agent.dev.init_script
}
