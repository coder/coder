terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_parameter" "color" {
  name      = "color"
  type      = "string"
  form_type = "dropdown"
  mutable   = true
  default   = "red"

  option {
    name  = "Red"
    value = "red"
  }

  option {
    name  = "Blue"
    value = "blue"
  }
}
