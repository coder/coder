terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "null_resource" "first" {}

resource "null_resource" "second" {}

resource "coder_metadata" "example" {
  resource_id = null_resource.second.id
  depends_on  = [null_resource.first]
  item {
    key   = "test"
    value = "value"
  }
}
