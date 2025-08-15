terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">=2.0.0"
    }
  }
}

resource "null_resource" "example" {}

resource "coder_metadata" "example" {
  resource_id = "non-existent-id"
  depends_on  = [null_resource.example]
  item {
    key   = "test"
    value = "value"
  }
}
