terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_secret" "gh" {
  env          = "GITHUB_TOKEN"
  help_message = "Add a GitHub PAT with env=GITHUB_TOKEN"
}
