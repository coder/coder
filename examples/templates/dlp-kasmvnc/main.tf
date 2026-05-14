// DLP-KasmVNC template. Declares a single `coder_dlp_policy` stanza that
// applies to every agent in any workspace built from this template. Mirrors
// the dlp-strict policy (every gate denied) except that the only allowed
// coder_app slug is "kasm-vnc", the slug created by the coder/kasmvnc
// registry module. Lets you confirm that a single allow-listed
// remote-desktop app works while everything else stays blocked.
//
// Base image is codercom/enterprise-base:ubuntu, which already provides a
// `coder` user with NOPASSWD sudo. XFCE and dbus-x11 are installed at
// agent-startup time, so no custom docker build step is required at the
// expense of a slow first boot. Subsequent restarts skip the apt install.
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

// Strict policy. Every gate denied; the only permitted coder_app slug is the
// one the kasmvnc module creates ("kasm-vnc"). Anything else, including the
// dashboard web terminal, the Ports tab, the noVNC Desktop button, and CLI
// peering, is blocked.
resource "coder_dlp_policy" "policy" {
  ssh_access             = false
  web_terminal_access    = false
  port_forwarding_access = false
  desktop_access         = false
  clipboard_access       = false
  allowed_applications   = ["kasm-vnc"]
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  # XFCE and the kasmvnc registry module's libdatetime-perl prereq are
  # installed by the container entrypoint before the agent starts (see
  # docker_container.workspace below). Nothing left for the agent itself
  # to do at startup.

  # Hide the VS Code Desktop, SSH, and port-forwarding helper buttons since
  # they are not gated by coder_dlp_policy. Keep web_terminal and desktop
  # visible so the dashboard's PTY and noVNC buttons are reachable; the
  # corresponding dlp_policy gates enforce the actual access decisions when
  # the user clicks them. The Desktop button targets the portabledesktop
  # module below, which is gated and not the kasmvnc app.
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
// No Terraform module is required. desktop_access=false on the policy
// above blocks the dashboard's /desktop endpoint, while the kasm-vnc
// coder_app remains reachable.
// KasmVNC registry module. Creates a single coder_app with slug "kasm-vnc"
// and a coder_script that installs and runs the server. subdomain = false
// makes the app reachable via path-based URLs, which is the access mode that
// works in nested Coder dev setups without wildcard DNS.
module "kasmvnc" {
  count               = data.coder_workspace.me.start_count
  source              = "registry.coder.com/coder/kasmvnc/coder"
  version             = "~> 1.2"
  agent_id            = coder_agent.main.id
  desktop_environment = "xfce"
  subdomain           = false
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-base:ubuntu"
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name
  # Pre-install everything the kasmvnc registry module's coder_script
  # depends on, before the agent (and therefore kasm) starts:
  #   * `apt-get update` populates the cache so kasm's apt-get install can
  #     resolve libdatetime-perl. Otherwise kasm sees an apparently fresh
  #     `/var/lib/apt/lists/partial` directory, skips its own update, and
  #     fails with "Unable to locate package libdatetime-perl".
  #   * `libdatetime-perl` is the kasmvnc package's runtime dependency.
  #   * `xfce4`, `xfce4-terminal`, and `dbus-x11` provide the XFCE desktop.
  #     Without these, kasmvncserver's `select-de.sh xfce` aborts with
  #     "'xfce': Desktop Environment not installed".
  # Subsequent restarts re-run this; apt-get install is a no-op when the
  # packages are already present.
  entrypoint = [
    "sh", "-c",
    "sudo apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libdatetime-perl xfce4 xfce4-terminal dbus-x11 && exec sh -c \"$0\" \"$@\"",
    coder_agent.main.init_script,
  ]
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
}
