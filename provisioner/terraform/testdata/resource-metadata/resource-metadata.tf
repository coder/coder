terraform {
  required_providers {
    coder = {
      source = "coder/coder"
      // TODO: update terraform-provider-coder before merge.
      version = "= 0.6.18-rc"
    }
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  metadata {
    key          = "process_count"
    display_name = "Process Count"
    cmd          = "ps -ef | wc -l"
    interval     = "1s"
  }
}

resource "null_resource" "about" {}

resource "coder_metadata" "about_info" {
  resource_id = null_resource.about.id
  hide        = true
  icon        = "/icon/server.svg"
  daily_cost  = 29
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
