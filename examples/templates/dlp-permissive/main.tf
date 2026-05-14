// DLP-permissive template. Declares a single `coder_dlp_policy` stanza that
// applies to every agent in any workspace built from this template, with
// every gate on. Pair this with `examples/templates/dlp-strict` to compare
// baseline, "fully open" behavior against the locked-down variant.
//
// Requires coder/coder built from the scott/x/dlp-prototype branch and a
// terraform-provider-coder binary that exposes `coder_dlp_policy`. Configure
// `dev_overrides` in ~/.terraformrc so Terraform picks up the local provider
// build.

terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

locals {
  username = data.coder_workspace_owner.me.name
}

provider "docker" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

// Permissive policy. Every gate is on, and every coder_app slug declared
// below is in `allowed_applications`. Any blocked operation observed from a
// workspace built on this template indicates a bug, not policy enforcement.
resource "coder_dlp_policy" "policy" {
  ssh_access             = true
  web_terminal_access    = true
  port_forwarding_access = true
  desktop_access         = true
  clipboard_access       = true
  allowed_applications   = ["code-server", "helloworld"]
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  startup_script = <<-EOT
    set -e
    # code-server: dashboard reaches it via the "code-server" coder_app slug.
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
    # helloworld: simple HTTP server on a different port. Used to verify that
    # `allowed_applications` admits the "helloworld" slug and that
    # `port_forwarding_access` permits the dashboard Ports tab.
    (cd /tmp && python3 -m http.server 8000 >/tmp/helloworld.log 2>&1) &
    # zutty: pulled in transitively by GTK/x-terminal-emulator. Hide it
    # from the dock so the only terminal launcher is the bundled xterm.
    if [ -f /usr/share/applications/zutty.desktop ] && ! grep -q '^NoDisplay=true' /usr/share/applications/zutty.desktop; then
      sudo tee -a /usr/share/applications/zutty.desktop >/dev/null <<'ZUTTY'
NoDisplay=true
ZUTTY
    fi
  EOT

  # Match the strict and kasmvnc templates: hide the VS Code Desktop, SSH,
  # and port-forwarding helper buttons but keep the web terminal and desktop
  # buttons. On a permissive policy these would all work; hiding them keeps
  # the surface consistent across the dlp-* templates.
  display_apps {
    vscode                 = false
    vscode_insiders        = false
    web_terminal           = true
    ssh_helper             = false
    port_forwarding_helper = false
    desktop                = true
  }
}

// portabledesktop is installed by the agent bootstrap when the dev server
// was built with BUNDLE_PORTABLEDESKTOP=1; the binary lands at
// ~/.local/bin/portabledesktop and the agent's PATH is updated to find it.
// No Terraform module is required.
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  url          = "http://localhost:13337/?folder=/home/${local.username}"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"
  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

resource "coder_app" "helloworld" {
  agent_id     = coder_agent.main.id
  slug         = "helloworld"
  display_name = "Hello World"
  url          = "http://localhost:8000/"
  subdomain    = false
  share        = "owner"
}

resource "docker_image" "main" {
  name = "codercom/enterprise-base:ubuntu"
}

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = docker_image.main.image_id
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}
