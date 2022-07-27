terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.4"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
}

resource "null_resource" "about" {}

resource "coder_metadata" "about_info" {
  resource_id = null_resource.about.id
  pair {
    key = "hello"
    value = "world"
  }
  pair {
    key = "null"
  }
  pair {
    key = "empty"
    value = ""
  }
  pair {
    key = "secret"
    value = "squirrel"
    sensitive = true
  }
}
