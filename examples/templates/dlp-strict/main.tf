// DLP-strict template. Every gate on `coder_dlp_policy` is denied except for
// the "code-server" coder_app slug, which remains in `allowed_applications`
// so workspace users still have one usable entry point. Pair this with
// `examples/templates/dlp-permissive` to compare locked-down vs baseline
// behavior.
//
// Expected denials when accessing a workspace built on this template:
//   - `coder ssh` / `coder port-forward`: 403 from
//     /api/v2/workspaceagents/.../coordinate (ssh_access=false).
//   - Dashboard web terminal: 403 (web_terminal_access=false).
//   - Dashboard Ports tab: 403 (port_forwarding_access=false).
//   - Dashboard Desktop button: 403 (desktop_access=false).
//   - "helloworld" coder_app: 403 (slug not in allowed_applications).
//
// The "code-server" coder_app continues to load because its slug is in
// allowed_applications and the dashboard app proxy is not gated by
// port_forwarding_access.
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

// Strict policy. Every gate is denied; only the "code-server" coder_app slug
// is allowed to load through the dashboard app proxy. The "helloworld" app
// is intentionally still defined so its blocked load can be observed.
resource "coder_dlp_policy" "policy" {
  ssh_access             = false
  web_terminal_access    = false
  port_forwarding_access = false
  desktop_access         = false
  allowed_applications   = ["code-server"]
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
    # helloworld: simple HTTP server. Reachable from inside the container but
    # blocked by allowed_applications via the dashboard app proxy.
    (cd /tmp && python3 -m http.server 8000 >/tmp/helloworld.log 2>&1) &
  EOT

  # Hide the VS Code Desktop, SSH, and port-forwarding helper buttons since
  # they are not gated by coder_dlp_policy. Keep web_terminal and desktop
  # visible so the dashboard's PTY and noVNC buttons are reachable; the
  # corresponding dlp_policy gates enforce the actual access decisions when
  # the user clicks them.
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
// connects to. Lets a user click the gated button and observe the 403 from
// the desktop_access denial.
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
