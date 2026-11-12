terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
module "grandchild" {
  for_each = toset(["one"])
  source   = "./grandchild"
  agent_id = var.agent_id
}
