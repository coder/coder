variable "script" {
  type = string
}

data "null_data_source" "script" {
  inputs = {
    script = var.script
  }
}

resource "null_resource" "example" {
  depends_on = [
    data.null_data_source.script
  ]
}
