terraform {
  required_providers {
    coder = {
      # coder_subagent_execution is not in a released provider yet. Use the
      # provider branch that ships the resource together with a Coder
      # deployment built from the matching server branch. The version is
      # intentionally unpinned so a locally built provider is picked up.
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

locals {
  # The one directory the human owner and the sandboxed child share. It is
  # the parent agent's working directory, the declared shared host path, and
  # the mount point of the persistent project volume.
  project_host_path = "/home/coder/project"
  # Where the same directory appears inside the sandbox. It must avoid the
  # directories the sandbox owns (/bin, /dev, /etc, /home, /opt/coder,
  # /proc, /run, /tmp), so it is not under the child's home.
  project_child_path = "/workspace/project"

  # The parent agent's init script, with a loopback access URL rewritten to
  # the Docker host gateway so a local deployment is reachable from the
  # container.
  parent_init_script = replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

provider "docker" {
  # Defaulting to null if the variable is an empty string lets us have an optional variable without having to set our own default
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# The human's parent agent. Its directory is the project root the shared
# path policy judges the declaration against: a declared shared host path
# has to resolve inside this directory.
resource "coder_agent" "main" {
  arch      = data.coder_provisioner.me.arch
  os        = "linux"
  directory = local.project_host_path

  startup_script = <<-EOT
    set -e

    # Prepare user home with default files on first start. The home
    # directory is container-local in this template: only the project
    # directory is persisted, and only the project directory is shared with
    # the sandboxed child.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    # Anything placed here is visible to the sandboxed child by design.
    echo "shared project directory: ${local.project_host_path}"
  EOT

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
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
    display_name = "Project Disk"
    key          = "2_project_disk"
    script       = "coder stat disk --path ${local.project_host_path}"
    interval     = 60
    timeout      = 1
  }

  metadata {
    display_name = "Sandbox Driver"
    key          = "3_sandbox_driver"
    # The driver runs as a child of the parent agent, so its presence is a
    # quick check that the sandbox is up without leaving the dashboard.
    script   = "pgrep -a bwrap >/dev/null 2>&1 && echo running || echo stopped"
    interval = 10
    timeout  = 3
  }

  metadata {
    display_name = "Sandbox Probes"
    key          = "4_sandbox_probes"
    # The probe report is written by the child into the shared project
    # directory, which is the only place the parent can read it from.
    script   = "sed -n 's/^SUMMARY: //p' ${local.project_host_path}/probe-results.txt 2>/dev/null | tail -n1 || true"
    interval = 30
    timeout  = 3
  }
}

# The nested isolated execution. Coder pre-creates a child workspace agent
# for this declaration, acquires its token privately, and invokes the vetted
# bubblewrap reference driver with a clean environment. The driver body is
# the checked-in script: it is read at plan time and travels with the
# template.
resource "coder_subagent_execution" "sandbox" {
  agent_id          = coder_agent.main.id
  name              = "sandbox"
  driver            = file("${path.module}/drivers/bwrap.sh")
  driver_protocol   = 1
  shared_host_path  = local.project_host_path
  shared_child_path = local.project_child_path
  startup_timeout   = 120
  restart_policy    = "never"
}

# A child-side workload, attached to the pre-created child agent. It runs
# inside the sandbox, so it may only use the BusyBox applets the driver
# exposes and the paths the sandbox provides.
resource "coder_script" "sandbox_project_web" {
  agent_id     = coder_subagent_execution.sandbox.subagent_id
  display_name = "Sandbox probes and project web server"
  icon         = "/icon/widgets.svg"
  run_on_start = true
  # The probe script ends by serving its own report in the foreground, so the
  # sandbox has a long-lived workload to observe. It deliberately does not
  # block login: the child web terminal stays usable while the script runs.
  start_blocks_login = false
  # The body is the checked-in probe script, prefixed with the shared path
  # this template declares, so the script and the template cannot disagree
  # about where the shared directory appears inside the sandbox.
  script = format(
    "PROBE_SHARED=%s\nexport PROBE_SHARED\n\n%s",
    local.project_child_path,
    file("${path.module}/scripts/probe.sh"),
  )
}

# The child's app. It is owner-shared: the sandbox is a private workload of
# this workspace, not something to publish.
resource "coder_app" "sandbox_project_web" {
  agent_id     = coder_subagent_execution.sandbox.subagent_id
  slug         = "sandbox-web"
  display_name = "Sandbox probe report"
  icon         = "/icon/widgets.svg"
  url          = "http://localhost:3000"
  share        = "owner"
  subdomain    = false
}

# The reference image. It carries the host tools the driver resolves (bwrap,
# jq, a statically linked BusyBox) and a CA bundle, and it creates the
# shared project directory owned by the workspace user.
resource "docker_image" "workspace" {
  name = "coder-linux-bwrap-subagent:latest"

  build {
    context    = "."
    dockerfile = "Dockerfile"
  }
}

# Only the project directory is persisted. The parent's home directory is
# container-local, which keeps the shared directory small and explicit.
resource "docker_volume" "project_volume" {
  name = "coder-${data.coder_workspace.me.id}-project"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
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
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = docker_image.workspace.image_id
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name

  # bubblewrap builds the sandbox from an unprivileged user namespace, so no
  # added capabilities are needed. Docker's default seccomp profile does
  # block the mount and pivot_root calls bubblewrap issues inside that
  # namespace, and on AppArmor hosts the docker-default profile denies
  # unprivileged user namespace creation outright. Both relaxations apply to
  # this container only; they do not weaken the boundary the driver builds
  # around the child.
  security_opts = [
    "seccomp=unconfined",
    "apparmor=unconfined",
  ]

  # The shared project directory must exist and be owned by the workspace
  # user before the parent agent handles its manifest: the shared path
  # policy resolves it during reconciliation, which happens before any
  # coder_script runs. The image creates it, the volume mount inherits that
  # ownership, and this mkdir is a cheap guard for a pre-existing volume.
  #
  # The parent fixtures run before the parent agent starts, so the markers
  # the sandbox probes look for already exist when the child comes up. Both
  # scripts are base64 encoded here so their quoting survives the shell that
  # unpacks them.
  entrypoint = [
    "sh", "-c",
    "mkdir -p ${local.project_host_path} && echo ${base64encode(file("${path.module}/scripts/parent-fixtures.sh"))} | base64 -d >/tmp/parent-fixtures.sh && sh /tmp/parent-fixtures.sh && echo ${base64encode(local.parent_init_script)} | base64 -d >/tmp/coder-init.sh && exec sh /tmp/coder-init.sh",
  ]
  env = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = local.project_host_path
    volume_name    = docker_volume.project_volume.name
    read_only      = false
  }

  # Add labels in Docker to keep track of orphan resources.
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
