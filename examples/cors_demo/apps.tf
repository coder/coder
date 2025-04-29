resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "VS Code"
  icon         = "${data.coder_workspace.me.access_url}/icon/code.svg"
  url          = "http://localhost:13337"
  share        = "owner"
  subdomain    = false
  open_in      = "window"
  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}
