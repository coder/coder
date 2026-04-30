terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_secret" "env_req" {
  env          = "GITHUB_TOKEN"
  help_message = "needs env"
}

data "coder_secret" "file_req" {
  file         = "~/.ssh/id_rsa"
  help_message = "needs file"
}
