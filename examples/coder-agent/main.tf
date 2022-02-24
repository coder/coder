terraform {
  required_providers {
    coder = {
      version = "0.2"
      source  = "coder.com/internal/coder"
    }
  }
}

provider "coder" {}

data ""

output "script_path" {
  value = coder.agent_script.linux
}
