variable "message" {
  type = string
}

output "hello_provisioner" {
  value = "Hello, provisioner: ${var.message}"
}
