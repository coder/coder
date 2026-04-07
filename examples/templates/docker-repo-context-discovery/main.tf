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

provider "docker" {}
provider "coder" {}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "repo_url" {
  type         = "string"
  name         = "repo_url"
  display_name = "Repository URL"
  description  = "URL of the Git repository to clone"
  mutable      = true
  default      = ""
}

data "coder_parameter" "context_service_url" {
  type         = "string"
  name         = "context_service_url"
  display_name = "Context Service URL"
  description  = "URL of an external context service. Leave empty to use the built-in mock response."
  mutable      = true
  default      = ""
}

data "coder_parameter" "context_service_token" {
  type         = "string"
  name         = "context_service_token"
  display_name = "Context Service Token"
  description  = "Bearer token for the context service (if required)."
  mutable      = true
  default      = ""
  sensitive    = true
}

locals {
  generated_context_root  = "~/.coder/generated-context"
  primary_repo_name       = regex("[^/]+$", trimsuffix(data.coder_parameter.repo_url.value, ".git"))
  context_on_clone_script = file("${path.module}/scripts/context-on-clone.sh")
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    # Install the shared discovery script so every clone path runs the same
    # repository context generation logic.
    mkdir -p ~/.coder/bin
    echo '${base64encode(local.context_on_clone_script)}' | base64 -d > ~/.coder/bin/context-on-clone.sh
    chmod +x ~/.coder/bin/context-on-clone.sh

    # Configure a git template so new runtime clones inherit the first-checkout
    # hook that refreshes repository-specific chat context.
    mkdir -p ~/.coder/git-templates/hooks
    cat > ~/.coder/git-templates/hooks/post-checkout <<'HOOK'
    #!/usr/bin/env bash
    # Trigger discovery only for the first checkout after clone so normal branch
    # switches do not regenerate context for an existing repository.
    PREV_HEAD="$1"
    if [ "$PREV_HEAD" != "0000000000000000000000000000000000000000" ]; then
      exit 0
    fi

    REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
    exec ~/.coder/bin/context-on-clone.sh "$REPO_DIR"
    HOOK
    chmod +x ~/.coder/git-templates/hooks/post-checkout
    git config --global init.templateDir ~/.coder/git-templates
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"

    # Point agent discovery at the generated context for the primary clone.
    CODER_AGENT_EXP_INSTRUCTIONS_DIRS = "${local.generated_context_root}/${local.primary_repo_name}"
    CODER_AGENT_EXP_INSTRUCTIONS_FILE = "AGENTS.md"
    CODER_AGENT_EXP_SKILLS_DIRS       = "${local.generated_context_root}/${local.primary_repo_name}/.agents/skills"
    CODER_AGENT_EXP_SKILL_META_FILE   = "SKILL.md"

    # Pass service configuration and the shared context root to clone hooks.
    CONTEXT_SERVICE_URL    = data.coder_parameter.context_service_url.value
    CONTEXT_SERVICE_TOKEN  = data.coder_parameter.context_service_token.value
    GENERATED_CONTEXT_ROOT = local.generated_context_root
  }

  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }
}

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  url      = data.coder_parameter.repo_url.value
  base_dir = "~"

  post_clone_script = file("${path.module}/scripts/context-on-clone.sh")
}

resource "docker_image" "main" {
  name = "codercom/enterprise-base:ubuntu"
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"

  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count      = data.coder_workspace.me.start_count
  image      = docker_image.main.image_id
  name       = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname   = data.coder_workspace.me.name
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}",
  ]

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
