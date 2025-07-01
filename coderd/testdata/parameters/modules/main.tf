terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.5.3"
    }
  }
}

module "jetbrains_gateway" {
  source = "jetbrains_gateway"
}

data "coder_parameter" "region" {
  name         = "region"
  display_name = "Where would you like to travel to next?"
  type         = "string"
  form_type    = "dropdown"
  mutable      = true
  default      = "na"
  order        = 1000

  option {
    name  = "North America"
    value = "na"
  }

  option {
    name  = "South America"
    value = "sa"
  }

  option {
    name  = "Europe"
    value = "eu"
  }

  option {
    name  = "Africa"
    value = "af"
  }

  option {
    name  = "Asia"
    value = "as"
  }
}
