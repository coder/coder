terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_parameter" "use_github" {
  name    = "use_github"
  type    = "bool"
  default = "false"
  mutable = true
}

data "coder_secret" "gh" {
  count        = data.coder_parameter.use_github.value == "true" ? 1 : 0
  env          = "GITHUB_TOKEN"
  help_message = "Add a GitHub PAT"
}
