variable "script" {
  type = string
}

resource "null_resource" "example" {
  depends_on = [
    var.script
  ]
}
