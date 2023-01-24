terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.6.7"
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
  delay_login_until_ready = false
}

resource "coder_agent" "dev3" {
  os                      = "windows"
  arch                    = "arm64"
  troubleshooting_url     = "https://coder.com/troubleshoot"
  delay_login_until_ready = true
}

resource "null_resource" "dev" {
  depends_on = [
    coder_agent.dev1,
    coder_agent.dev2,
    coder_agent.dev3
  ]
}
