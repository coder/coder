terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

# A list(string) with options and no form_type is a radio, so each option value is a whole list.
data "coder_parameter" "regions" {
  name    = "regions"
  type    = "list(string)"
  mutable = true
  default = jsonencode(["eu-west-1"])

  option {
    name  = "US"
    value = jsonencode(["us-east-1", "us-west-2"])
  }

  option {
    name  = "EU"
    value = jsonencode(["eu-west-1"])
  }
}
