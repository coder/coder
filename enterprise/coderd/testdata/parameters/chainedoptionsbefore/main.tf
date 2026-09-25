terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

data "coder_parameter" "region" {
  name      = "region"
  type      = "string"
  form_type = "dropdown"
  mutable   = true
  default   = "us"

  option {
    name  = "US"
    value = "us"
  }

  option {
    name  = "EU"
    value = "eu"
  }
}

# removing a region option invalidates what was selected before.
data "coder_parameter" "instance" {
  name      = "instance"
  type      = "string"
  form_type = "dropdown"
  mutable   = true
  default   = "${data.coder_parameter.region.value}-small"

  option {
    name  = "Small"
    value = "${data.coder_parameter.region.value}-small"
  }

  option {
    name  = "Large"
    value = "${data.coder_parameter.region.value}-large"
  }
}
