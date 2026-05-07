// DLP-permissive template. Every gate on `coder_dlp_policy` is allowed.
// Pair this with `examples/templates/dlp-strict` to compare baseline,
// "fully open" behavior against the locked-down variant.
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
  allowed_applications   = ["code-server", "helloworld"]
}

resource "coder_agent" "main" {
  arch       = data.coder_provisioner.me.arch
  os         = "linux"
  dlp_policy = coder_dlp_policy.policy.id

  startup_script = <<-EOT
    set -e
    # code-server: dashboard reaches it via the "code-server" coder_app slug.
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
    # helloworld: simple HTTP server on a different port. Used to verify that
    # `allowed_applications` admits the "helloworld" slug and that
    # `port_forwarding_access` permits the dashboard Ports tab.
    (cd /tmp && python3 -m http.server 8000 >/tmp/helloworld.log 2>&1) &
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

// portabledesktop: installs the noVNC server the dashboard's Desktop button
// connects to. Mirrors the dogfood template (dogfood/coder/main.tf) so the
// Desktop button has something to display.
module "portabledesktop" {
  source   = "dev.registry.coder.com/coder/portabledesktop/coder"
  version  = "0.1.0"
  agent_id = coder_agent.main.id
}

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
