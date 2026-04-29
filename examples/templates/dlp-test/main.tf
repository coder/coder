// DLP test template. A docker-based workspace whose agent's DLP policy is
// driven by rich parameters so you can flip each gate from the Coder UI
// without re-pushing the template version.
//
// Defaults are fully permissive (every flag true, every app allowed) so the
// first build always works. Switch a parameter to "deny" and rebuild to
// exercise the matching enforcement gate in coderd.
//
// Requires a coder/coder server built from the scott/x/dlp-prototype branch
// AND a terraform-provider-coder binary that exposes `coder_dlp_policy`. Use
// `dev_overrides` in ~/.terraformrc to point at a local build of the
// scott/x/dlp-prototype branch of terraform-provider-coder.

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

// ---------------------------------------------------------------------------
// DLP parameters. Each rebuild reads these and rewrites the policy.

data "coder_parameter" "ssh_access" {
  name         = "ssh_access"
  display_name = "Allow CLI access (ssh, port-forward, cp, speedtest)"
  description  = "When false, /api/v2/workspaceagents/.../coordinate returns 403. Blocks all coder CLI peering."
  type         = "bool"
  default      = "true"
  mutable      = true
}

data "coder_parameter" "web_terminal_access" {
  name         = "web_terminal_access"
  display_name = "Allow dashboard web terminal"
  description  = "When false, the dashboard web terminal returns 403."
  type         = "bool"
  default      = "true"
  mutable      = true
}

data "coder_parameter" "port_forwarding_access" {
  name         = "port_forwarding_access"
  display_name = "Allow dashboard Ports tab"
  description  = "When false, accessing a workspace port via the dashboard returns 403. Does NOT affect `coder port-forward` (CLI) — that is gated by ssh_access."
  type         = "bool"
  default      = "true"
  mutable      = true
}

data "coder_parameter" "allowed_apps_mode" {
  name         = "allowed_apps_mode"
  display_name = "Allowed coder_app slugs"
  description  = "Selects which coder_app slugs are permitted via the dashboard app proxy."
  type         = "string"
  default      = "all"
  mutable      = true
  option {
    name  = "Allow both apps"
    value = "all"
  }
  option {
    name  = "Allow only code-server"
    value = "code-server-only"
  }
  option {
    name  = "Block all"
    value = "none"
  }
}

locals {
  allowed_apps = {
    "all"              = ["code-server", "helloworld"]
    "code-server-only" = ["code-server"]
    "none"             = []
  }[data.coder_parameter.allowed_apps_mode.value]
}

// ---------------------------------------------------------------------------
// Policy + agent.

resource "coder_dlp_policy" "policy" {
  ssh_access             = data.coder_parameter.ssh_access.value
  web_terminal_access    = data.coder_parameter.web_terminal_access.value
  port_forwarding_access = data.coder_parameter.port_forwarding_access.value
  allowed_applications   = local.allowed_apps
}

resource "coder_agent" "main" {
  arch       = data.coder_provisioner.me.arch
  os         = "linux"
  dlp_policy = coder_dlp_policy.policy.id

  startup_script = <<-EOT
    set -e
    # code-server: dashboard reaches it via coder_app slug "code-server".
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
    # helloworld: simple HTTP server on a different port. Used to verify that
    # `allowed_applications` filters by slug, and that `port_forwarding_access`
    # gates the dashboard Ports tab.
    (cd /tmp && python3 -m http.server 8000 >/tmp/helloworld.log 2>&1) &
  EOT
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

// ---------------------------------------------------------------------------
// Container.

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
