plugin "terraform" {
    enabled = true
    version = "0.2.1"
    source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}

rule "terraform_standard_module_structure" {
  enabled = false
}

plugin "aws" {
    enabled = true
    version = "0.18.0"
    source  = "github.com/terraform-linters/tflint-ruleset-aws"
}

plugin "google" {
    enabled = true
    version = "0.21.0"
    source  = "github.com/terraform-linters/tflint-ruleset-google"
}

plugin "azurerm" {
    enabled = true
    version = "0.19.0"
    source  = "github.com/terraform-linters/tflint-ruleset-azurerm"
}

