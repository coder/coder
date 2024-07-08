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
  display_apps {
    vscode                 = false
    vscode_insiders        = true
    web_terminal           = true
    port_forwarding_helper = false
    ssh_helper             = false
  }
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.main
  ]
}
