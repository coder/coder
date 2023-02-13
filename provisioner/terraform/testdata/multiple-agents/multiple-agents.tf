terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.12"
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
  login_before_ready      = true
  shutdown_script         = "echo bye bye"
  shutdown_script_timeout = 30
}

resource "coder_agent" "dev3" {
  os                  = "windows"
  arch                = "arm64"
  troubleshooting_url = "https://coder.com/troubleshoot"
  login_before_ready  = false
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1,
    coder_agent.dev2,
    coder_agent.dev3
  ]
}
