terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.8.3"
    }
  }
}

resource "coder_agent" "dev1" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_agent" "dev2" {
  os                      = "darwin"
  arch                    = "amd64"
  connection_timeout      = 1
  motd_file               = "/etc/motd"
  startup_script_timeout  = 30
  startup_script_behavior = "non-blocking"
  shutdown_script         = "echo bye bye"
  shutdown_script_timeout = 30
}

resource "coder_agent" "dev3" {
  os                      = "windows"
  arch                    = "arm64"
  troubleshooting_url     = "https://coder.com/troubleshoot"
  startup_script_behavior = "blocking"
}

resource "coder_agent" "dev4" {
  os   = "linux"
  arch = "amd64"
  # Test deprecated login_before_ready=false => startup_script_behavior=blocking.
  login_before_ready = false
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1,
    coder_agent.dev2,
    coder_agent.dev3,
    coder_agent.dev4
  ]
}
