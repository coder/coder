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

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

variable "claude_code_version" {
  default     = "latest"
  description = "Version of the Claude Code CLI to install."
  type        = string
}

variable "ai_gateway_provider" {
  default     = "anthropic"
  description = <<-EOT
    Name of the AI Gateway provider to send Anthropic traffic to. The gateway
    serves each provider under its configured name, so this must match the
    provider name on the deployment, not the provider type.
  EOT
  type        = string
}

variable "claude_model" {
  default     = ""
  description = <<-EOT
    (Optional) Model ID to pin Claude Code to via ANTHROPIC_MODEL. Leave empty
    to let Claude Code choose, which requires the provider to accept its
    default model ID.
  EOT
  type        = string
}

variable "access_url_override" {
  default     = ""
  description = <<-EOT
    (Optional) Deployment URL to use inside the workspace container. Set this
    when the container cannot resolve the deployment access URL, for example a
    local development deployment reachable only on a specific host address.
  EOT
  type        = string
}

provider "docker" {
  # Defaulting to null if the variable is an empty string lets us have an optional variable without having to set our own default
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  # The container reaches the deployment through the Docker host gateway when
  # the access URL points at the host loopback address.
  access_url = var.access_url_override != "" ? var.access_url_override : replace(data.coder_workspace.me.access_url, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")

  # Claude Code sends Anthropic traffic here, so every request is recorded as an
  # AI Gateway interception.
  anthropic_base_url = "${local.access_url}/api/v2/ai-gateway/${var.ai_gateway_provider}"

  # The annotations toolset exposes only coder_annotate_interception and sends
  # instructions describing when to call it.
  mcp_url = "${local.access_url}/api/experimental/mcp/http?toolset=annotations"

  # Claude Code namespaces MCP tools as mcp__<server>__<tool>.
  annotation_tool = "mcp__coder-ai-gateway__coder_annotate_interception"

  # Merges the annotation tool into permissions.allow. Claude Code writes
  # settings.json itself, so the file usually exists already.
  settings_merge_script = <<-EOT
    import json
    import os
    import sys

    path = os.path.expanduser("~/.claude/settings.json")
    tool = sys.argv[1]
    try:
        with open(path) as handle:
            settings = json.load(handle)
    except (OSError, ValueError):
        settings = {}
    if not isinstance(settings, dict):
        settings = {}
    permissions = settings.setdefault("permissions", {})
    allow = permissions.setdefault("allow", [])
    if tool not in allow:
        allow.append(tool)
    with open(path, "w") as handle:
        json.dump(settings, handle, indent=2)
        handle.write("\n")
  EOT

  # Injected as user memory and as an appended system prompt. Server
  # instructions alone do not reliably prompt a call.
  annotation_context = <<-EOT
    # Coder AI Gateway work context

    This session runs through the Coder AI Gateway, which records every
    request. The `${local.annotation_tool}` tool attaches the work being done
    to that record.

    Call `${local.annotation_tool}` when any of the following is true:

    - The user names an issue, for example `PLAT-666` or `ENG-1234`. Pass it in
      `linear_issue_ids`.
    - You learn the repository or branch you are working in. Pass `repo` as
      `owner/name` and `branch` as the current branch.
    - A pull request is opened or referenced. Pass its URL in `github_pr_urls`.

    Rules:

    - Call it as soon as you learn a value, without being asked, and again
      whenever a value changes or a new issue or pull request appears.
    - Pass only the fields you are confident about. Omitted fields keep their
      previous value, and issues and pull requests accumulate.
    - Never guess values and never ask the user for them.
    - Do not mention the tool or the annotation to the user; just call it and
      carry on with the actual task.
  EOT
}

resource "coder_agent" "main" {
  arch           = data.coder_provisioner.me.arch
  os             = "linux"
  startup_script = <<-EOT
    set -e

    # Prepare user home with default files on first start.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"

    # Route Claude Code through the Coder AI Gateway, authenticated as the
    # workspace owner.
    ANTHROPIC_BASE_URL   = local.anthropic_base_url
    ANTHROPIC_AUTH_TOKEN = data.coder_workspace_owner.me.session_token
    ANTHROPIC_MODEL      = var.claude_model
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
}

# Installs the Claude Code CLI, registers the Coder MCP server that hosts the
# interception annotation tool, and writes the annotation instructions as user
# memory. The session token is written into the user-scope MCP configuration at
# ~/.claude.json.
resource "coder_script" "claude_code" {
  count              = data.coder_workspace.me.start_count
  agent_id           = coder_agent.main.id
  display_name       = "Claude Code"
  icon               = "/icon/claude.svg"
  run_on_start       = true
  start_blocks_login = false
  script             = <<-EOT
    #!/usr/bin/env bash
    set -euo pipefail

    export PATH="$HOME/.local/bin:$PATH"

    if ! command -v claude >/dev/null 2>&1; then
      curl -fsSL https://claude.ai/install.sh | bash -s -- "${var.claude_code_version}"
    fi

    # Ensure interactive shells find the CLI.
    if ! grep -q 'HOME/.local/bin' "$HOME/.bashrc" 2>/dev/null; then
      echo 'export PATH="$HOME/.local/bin:$PATH"' >>"$HOME/.bashrc"
    fi

    # The owner session token is reissued on every build, so the existing entry
    # is replaced rather than left in place; add-json refuses to overwrite.
    claude mcp remove --scope user coder-ai-gateway >/dev/null 2>&1 || true
    claude mcp add-json --scope user coder-ai-gateway "$(
      printf '{"type":"http","url":"%s","headers":{"Coder-Session-Token":"%s"}}' \
        '${local.mcp_url}' '${data.coder_workspace_owner.me.session_token}'
    )"

    mkdir -p "$HOME/.claude"

    # base64 keeps the markdown clear of shell quoting.
    context_file="$HOME/.claude/coder-annotation-context.md"
    echo '${base64encode(local.annotation_context)}' | base64 -d >"$context_file"

    # ~/.claude/CLAUDE.md is user memory and applies in every directory. It is
    # written only when absent or when this script wrote it before.
    memory_file="$HOME/.claude/CLAUDE.md"
    marker='<!-- coder-ai-gateway-annotations -->'
    if [ ! -f "$memory_file" ] || grep -qF "$marker" "$memory_file"; then
      {
        echo "$marker"
        cat "$context_file"
      } >"$memory_file"
    fi

    # Pre-approve the tool so annotating does not raise a permission prompt
    # mid-task.
    merge_script="$HOME/.claude/.coder-merge-settings.py"
    echo '${base64encode(local.settings_merge_script)}' | base64 -d >"$merge_script"
    python3 "$merge_script" '${local.annotation_tool}'
  EOT
}

resource "coder_app" "claude" {
  agent_id     = coder_agent.main.id
  slug         = "claude"
  display_name = "Claude Code"
  icon         = "/icon/claude.svg"
  open_in      = "slim-window"
  command      = <<-EOT
    #!/usr/bin/env bash
    set -e
    export PATH="$HOME/.local/bin:$PATH"
    context_file="$HOME/.claude/coder-annotation-context.md"
    if [ -f "$context_file" ]; then
      exec claude --append-system-prompt "$(cat "$context_file")"
    fi
    exec claude
  EOT
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/code-server/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  order    = 1
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/example-base:ubuntu"
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

  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
