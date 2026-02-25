terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

variable "agent_id" {
  type        = string
  description = "The ID of the Coder agent to attach the capture script to."
}

variable "no_trailer" {
  type        = bool
  default     = false
  description = "If true, do not inject Coder-Session trailer into commit messages."
}

variable "log_dir" {
  type        = string
  default     = ""
  description = "Custom directory for capture logs. Defaults to ~/coder-capture-logs/."
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_script" "coder_capture" {
  agent_id           = var.agent_id
  display_name       = "AI Session Capture"
  icon               = "/icon/git.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = templatefile("${path.module}/run.sh", {
    no_trailer = var.no_trailer
    log_dir    = var.log_dir
  })
}
