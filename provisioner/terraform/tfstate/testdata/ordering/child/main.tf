terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
# The key reads like the end of one module address and the start of another.
module "grandchild" {
  for_each = toset(["a\"].module.b[\"c"])
  source   = "./grandchild"
  agent_id = var.agent_id
}
