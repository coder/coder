# For interesting types of variables, check out the terraform docs:
# https://www.terraform.io/language/values/variables#declaring-an-input-variable
variable "message" {
  type = string
}

# We can use a "null_resource" to test resources without a cloud provider:
# https://www.terraform.io/language/resources/provisioners/null_resource
resource "null_resource" "minimal_resource" {

  # Note that Terraform's `provisioner` concept is generally an anti-pattern -
  # more info here: https://www.terraform.io/language/resources/provisioners/syntax
  # But it's helpful here for testing a resource.
  provisioner "local-exec" {
    command = "echo ${var.message}"
  }
}
