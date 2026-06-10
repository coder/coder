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

data "coder_provisioner" "me" {}

data "coder_workspace" "me" {}

data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
}

# 1) Simulates slow system preparation (package mirrors, base tooling).
#    Nothing else may rely on the network mirrors until this finishes.
resource "coder_script" "configure_mirrors" {
  agent_id           = coder_agent.main.id
  display_name       = "Configure mirrors"
  icon               = "/icon/database.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = <<-EOT
    set -e
    echo "Configuring package mirrors..."
    sleep 5
    echo "Mirrors ready."
  EOT
}

# 2) A stock registry module. It needs no modifications and contains no
#    coordination logic; ordering is declared below in coder_script_order.
module "git_clone" {
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  url      = "https://github.com/coder/coder"
}

# 3) A template-level script that requires the cloned repository.
resource "coder_script" "run_agent" {
  agent_id           = coder_agent.main.id
  display_name       = "Run AI agent"
  icon               = "/icon/code.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = <<-EOT
    set -e
    test -d "$HOME/coder" || { echo "repository missing" >&2; exit 1; }
    echo "Repository present:"
    ls "$HOME/coder" | head
  EOT
}

# 4) Independent script: not referenced by any rule, so it starts
#    immediately and runs in parallel with everything else.
resource "coder_script" "dotfiles" {
  agent_id           = coder_agent.main.id
  display_name       = "Dotfiles"
  icon               = "/icon/dotfiles.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = "echo 'dotfiles done'"
}

# 5) Always runs last, even when the AI agent step fails.
resource "coder_script" "report" {
  agent_id           = coder_agent.main.id
  display_name       = "Report status"
  icon               = "/icon/widgets.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = "echo 'startup finished'"
}

# Declarative execution order. Scripts referenced by a rule wait for the
# scripts they are ordered after; everything else stays parallel.
data "coder_script_order" "startup" {
  # The git-clone module may only start after the mirrors are configured.
  rule {
    run   = "module.git_clone"
    after = ["coder_script.configure_mirrors"]
  }

  # The AI agent needs the repository. requires defaults to "success",
  # so this script is skipped when the clone fails.
  rule {
    run   = "coder_script.run_agent"
    after = ["module.git_clone"]
  }

  # The report runs once the agent step is finished, even if it failed.
  rule {
    run      = "coder_script.report"
    after    = ["coder_script.run_agent"]
    requires = "completion"
  }
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
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
