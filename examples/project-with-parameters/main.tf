# For interesting types of variables, check out the terraform docs:
# https://www.terraform.io/language/values/variables#declaring-an-input-variable
variable "message" {
  type = string
}

# And refer to the docs on using expressions to make use of variables:
# https://www.terraform.io/language/expressions/strings#interpolation
output "hello_provisioner" {
  value = "Hello, provisioner: ${var.message}"
}
