# Cursor Personal Worker, minimal Coder template.
#
# One workspace per developer. The developer pastes their own personal
# Cursor API key on workspace creation, the startup script installs
# cursor-agent and starts a worker (no --pool) named after the Coder
# workspace owner. The worker registers as the developer's My
# Machines worker and shows up in their cursor.com agents dropdown.
#
# Companion doc: docs/ai-coder/cursor-self-hosted-workers/personal-workers.md

terraform {
  required_providers {
    coder  = { source = "coder/coder", version = ">= 2.4.1" }
    docker = { source = "kreuzwerker/docker" }
  }
}

provider "coder" {}
provider "docker" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "cursor_api_key" {
  name         = "cursor_api_key"
  display_name = "Cursor personal API key"
  description  = "Generate from cursor.com/dashboard, Integrations, API Keys."
  type         = "string"
  mutable      = true
}

data "coder_parameter" "git_repo_url" {
  name         = "git_repo_url"
  display_name = "Git repository URL"
  description  = "Repository this worker serves."
  type         = "string"
  default      = "https://github.com/coder/coder"
  mutable      = false
}

locals {
  worker_name    = "coder-${data.coder_workspace_owner.me.name}"
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
  dir  = "/home/coder"

  env = {
    CURSOR_API_KEY = data.coder_parameter.cursor_api_key.value
    GIT_REPO_URL   = data.coder_parameter.git_repo_url.value
    WORKER_NAME    = local.worker_name
  }

  startup_script = <<-EOT
    set -eu
    export PATH="$HOME/.local/bin:$PATH"

    REPO_DIR="$HOME/workspace"

    if ! command -v cursor-agent >/dev/null 2>&1; then
      curl -fsSL "https://cursor.com/install" | bash
      export PATH="$HOME/.local/bin:$PATH"
    fi

    if [ ! -d "$REPO_DIR/.git" ]; then
      rm -rf "$REPO_DIR"
      git clone "$GIT_REPO_URL" "$REPO_DIR"
    else
      cd "$REPO_DIR"
      git remote set-url origin "$GIT_REPO_URL"
      git fetch --prune origin
    fi
    cd "$REPO_DIR"
    git config --global user.name  "${coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)}"
    git config --global user.email "${data.coder_workspace_owner.me.email}"

    nohup cursor-agent --api-key "$CURSOR_API_KEY" worker \
      --worker-dir       "$REPO_DIR" \
      --management-addr  ":8080" \
      --name             "$WORKER_NAME" \
      start >> "$HOME/cursor-agent.log" 2>&1 &
    echo "cursor-agent personal worker spawned as $WORKER_NAME"
  EOT

  metadata {
    display_name = "Worker process"
    key          = "0_worker_process"
    interval     = 10
    timeout      = 2
    script       = <<-EOS
      if pgrep -f "cursor-agent .* worker" >/dev/null 2>&1; then echo running
      else echo stopped; fi
    EOS
  }

  metadata {
    display_name = "Ready (idle)"
    key          = "1_ready"
    interval     = 5
    timeout      = 3
    script       = <<-EOS
      val=$(curl -fs --max-time 2 http://127.0.0.1:8080/metrics 2>/dev/null \
            | awk '/^cursor_self_hosted_worker_session_active /{print $2}')
      case "$val" in
        0) echo idle ;;
        1) echo in-use ;;
        *) echo unknown ;;
      esac
    EOS
  }
}

resource "docker_volume" "home" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle { ignore_changes = all }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/oss-dogfood:latest"
  name     = local.container_name
  hostname = data.coder_workspace.me.name
  user     = "coder"

  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home.name
    read_only      = false
  }
}
