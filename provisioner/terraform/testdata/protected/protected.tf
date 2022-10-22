terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.5.3"
    }
  }
}


resource "null_resource" "oops" {}

resource "null_resource" "whew" {
    lifecycle {
        ignore_changes = all
    }
}

