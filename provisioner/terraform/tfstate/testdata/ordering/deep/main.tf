terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
module "child" {
  source   = "../child"
  agent_id = var.agent_id
}
