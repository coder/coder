terraform {
  required_providers {
    coder = { source = "coder/coder" }
  }
}
variable "agent_id" { type = string }
data "coder_workspace" "me" {}
resource "coder_script" "deep" {
  agent_id     = var.agent_id
  display_name = "deep"
  script       = "echo deep"
  run_on_start = true
}
