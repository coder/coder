terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.5.0"
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
  hide        = true
  icon        = "/icon/server.svg"
  item {
    key   = "hello"
    value = "world"
  }
  item {
    key = "null"
  }
  item {
    key   = "empty"
    value = ""
  }
  item {
    key       = "secret"
    value     = "squirrel"
    sensitive = true
  }
}
