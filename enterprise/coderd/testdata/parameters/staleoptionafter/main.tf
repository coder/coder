terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

# "red" is no longer offered
# workspace that selected it holds a value that is not an option anymore.
data "coder_parameter" "color" {
  name      = "color"
  type      = "string"
  form_type = "dropdown"
  mutable   = true
  default   = "blue"

  option {
    name  = "Blue"
    value = "blue"
  }

  option {
    name  = "Green"
    value = "green"
  }
}
