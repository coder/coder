# Terraform merges the files of a module, so the state does not depend on
# which file declares a data source or resource.
data "coder_parameter" "region" {
  name    = "region"
  default = "us"
  option {
    name  = "US"
    value = "us"
  }
  option {
    name  = "EU"
    value = "eu"
  }
}

data "coder_parameter" "cpus" {
  name    = "cpus"
  type    = "number"
  default = 2
  validation {
    min = 1
    max = 8
  }
}
