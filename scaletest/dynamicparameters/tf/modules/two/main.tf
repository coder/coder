terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.5.3"
    }
  }
}

variable "one_value" {
  description = "The value from the 'one' parameter"
  type        = string
}

data "coder_parameter" "two" {
  name         = "Two"
  display_name = "Level Two"
  description  = "This is the second level."

  type      = "string"
  form_type = "textarea"
  order     = 3
  mutable   = true

  default = trim(var.one_value, "[\"]")
}

output "two_value" {
  description = "The value of the 'two' parameter"
  value       = data.coder_parameter.two.value
}
