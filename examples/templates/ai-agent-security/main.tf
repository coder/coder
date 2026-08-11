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

variable "workspace_image" {
  description = <<-EOT
    Workspace image. The default has neither the Docker CLI nor the
    coder-sandbox binary, so it cannot run either sandbox backend. Point
    this at an image that carries the tooling for the backend you intend to
    demo; see the README.
  EOT
  type        = string
  default     = "codercom/example-base:ubuntu"
}

variable "sandbox_image" {
  description = "OCI image booted inside the coder/sandbox microVM."
  type        = string
  default     = "ubuntu:latest"
}

variable "sandbox_memory_mib" {
  description = "Guest memory for the coder/sandbox microVM, in MiB."
  type        = number
  default     = 1024
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# The server looks for this exact parameter name to decide whether a
# workspace is AI-designated. Designation is sticky: setting this back to
# false on a later build does NOT restore the owner's credentials, because
# a parameter edit must never be able to un-starve an AI workspace.
data "coder_parameter" "coder_ai_agent" {
  name         = "coder_ai_agent"
  display_name = "AI-designated workspace"
  description  = <<-EOT
    Run this workspace under a dedicated AI agent identity. Its agents
    receive no owner credentials (no user secrets, no external auth, no Git
    SSH key, no owner session token) and get a workspace-scoped AI token
    instead. This cannot be undone by a later build.
  EOT
  type         = "bool"
  default      = "true"
  mutable      = true
  order        = 1
}

data "coder_parameter" "confinement" {
  name         = "confinement"
  display_name = "Network confinement"
  description  = <<-EOT
    off: no egress control.
    proxy: the agent routes through a local policy proxy (advisory: a
    process that ignores proxy variables escapes it).
    netns: the agent runs inside a network namespace whose only route is
    the proxy (structural). Requires NET_ADMIN and iproute2 in the image;
    it falls back to proxy mode and reports degraded if setup fails.
  EOT
  type         = "string"
  default      = "proxy"
  mutable      = true
  order        = 2

  option {
    name  = "Proxy only (advisory)"
    value = "proxy"
  }
  option {
    name  = "Network namespace (structural)"
    value = "netns"
  }
  option {
    name  = "Off"
    value = "off"
  }
}

data "coder_parameter" "enable_sandbox" {
  name         = "enable_sandbox"
  display_name = "Run an AI sandbox"
  description  = <<-EOT
    Have the workspace agent create a Docker sandbox holding a second,
    AI-bound agent. Requires the Docker CLI inside the workspace image and
    the mounted Docker socket enabled below.
  EOT
  type         = "bool"
  default      = "false"
  mutable      = true
  order        = 3
}

data "coder_parameter" "sandbox_backend" {
  name         = "sandbox_backend"
  display_name = "Sandbox backend"
  description  = <<-EOT
    microvm: coder/sandbox boots a libkrun microVM. Strongest isolation,
    but it requires /dev/kvm, the coder-sandbox binary in the workspace
    image, and a preseeded microsandbox runtime. Egress is enforced and
    recorded by coder/sandbox itself, so the platform egress event stream
    stays empty for the sandbox.
    docker: a container on an internal Docker network. Weaker isolation,
    far fewer host requirements, and its egress flows through the platform
    proxy so they appear in the platform audit stream.
  EOT
  type         = "string"
  default      = "microvm"
  mutable      = true
  order        = 4

  option {
    name  = "microVM (coder/sandbox)"
    value = "microvm"
  }
  option {
    name  = "Docker container"
    value = "docker"
  }
}

data "coder_parameter" "sandbox_allow" {
  name         = "sandbox_allow"
  display_name = "Sandbox egress allowlist"
  description  = <<-EOT
    Extra hosts the microVM sandbox may reach, comma separated. The coderd
    host is always allowed so the child agent can connect. A leading dot
    matches subdomains, for example .github.com. Ignored by the Docker
    backend, whose egress is governed by the template egress policy
    instead.
  EOT
  type         = "string"
  default      = ""
  mutable      = true
  order        = 6
}

data "coder_parameter" "sandbox_enforcement" {
  name         = "sandbox_enforcement"
  display_name = "Sandbox egress attestation"
  description  = <<-EOT
    What the sandbox script claims about its egress routing. This is an
    admin attestation that the platform records but cannot verify. The
    bundled script honors it: forced builds an internal Docker network with
    no route out, advisory only sets proxy variables.
  EOT
  type         = "string"
  default      = "forced"
  mutable      = true
  order        = 5

  option {
    name  = "forced"
    value = "forced"
  }
  option {
    name  = "advisory"
    value = "advisory"
  }
  option {
    name  = "none"
    value = "none"
  }
}

locals {
  username = data.coder_workspace_owner.me.name

  confinement     = data.coder_parameter.confinement.value
  sandbox_wanted  = data.coder_parameter.enable_sandbox.value
  sandbox_backend = data.coder_parameter.sandbox_backend.value
  sandbox_microvm = data.coder_parameter.enable_sandbox.value && data.coder_parameter.sandbox_backend.value == "microvm"
  sandbox_docker  = data.coder_parameter.enable_sandbox.value && data.coder_parameter.sandbox_backend.value == "docker"

  # Both script pairs are staged; the declaration selects one. Keeping both
  # on disk lets an operator switch backends by editing the parameter
  # without rebuilding the image.
  create_script  = local.sandbox_backend == "microvm" ? "sandbox-create-microvm.sh" : "sandbox-create.sh"
  destroy_script = local.sandbox_backend == "microvm" ? "sandbox-destroy-microvm.sh" : "sandbox-destroy.sh"

  # The sandbox proxy must be reachable from inside the sandbox container,
  # so it cannot bind the workspace's loopback address. The Docker bridge
  # reaches the workspace container on all interfaces.
  sandbox_proxy_address = "0.0.0.0:13337"

  # Scripts are staged by the container entrypoint rather than a startup
  # script, because the sandbox controller starts with the agent and must
  # not race a script that has not been written yet.
  staging_dir = "/tmp/coder-ai"

  agent_init = replace(
    coder_agent.main.init_script,
    "/localhost|127\\.0\\.0\\.1/",
    "host.docker.internal",
  )

  # Container environment, which is what the agent PROCESS sees. This is
  # deliberately not coder_agent.env: that lands in the agent manifest and
  # is injected into processes the agent spawns, not into the agent itself,
  # so the confinement and sandbox declarations would never be read.
  agent_process_env = concat(
    ["CODER_AGENT_TOKEN=${coder_agent.main.token}"],
    local.confinement == "off" ? [] : ["CODER_AGENT_CONFINE=${local.confinement}"],
    local.sandbox_wanted ? [
      "CODER_AI_SANDBOX_CREATE_SCRIPT=${local.staging_dir}/${local.create_script}",
      "CODER_AI_SANDBOX_DESTROY_SCRIPT=${local.staging_dir}/${local.destroy_script}",
      "CODER_AI_SANDBOX_NAME=demo",
      "CODER_AI_SANDBOX_EGRESS_ENFORCEMENT=${data.coder_parameter.sandbox_enforcement.value}",
      "CODER_AI_SANDBOX_PROXY_ADDRESS=${local.sandbox_proxy_address}",
    ] : [],
    # Consumed by the microVM script only. The allowlist is separate from
    # the template egress policy because coder/sandbox enforces the guest's
    # egress itself; see the README.
    local.sandbox_microvm ? [
      "CODER_AI_SANDBOX_ALLOW=${data.coder_parameter.sandbox_allow.value}",
      "CODER_AI_SANDBOX_IMAGE=${var.sandbox_image}",
      "CODER_AI_SANDBOX_MEMORY_MIB=${var.sandbox_memory_mib}",
    ] : [],
  )
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  startup_script = <<-EOT
    set -e

    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~ 2>/dev/null || true
      touch ~/.init_done
    fi
  EOT

  # Ordinary manifest environment. In an AI-designated workspace the owner
  # session token is suppressed server-side, so a template that wants a
  # token inside the workspace must use the scoped AI token instead. That
  # is exposed to Terraform by the provider work that is not landed yet, so
  # this template deliberately ships without a session token in the
  # environment.
  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
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
}

resource "docker_volume" "microsandbox_cache" {
  count = local.sandbox_microvm ? 1 : 0
  name  = "coder-${data.coder_workspace.me.id}-microsandbox"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
}

resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = var.workspace_image
  name     = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  hostname = data.coder_workspace.me.name

  # Stage the sandbox scripts and the agent init script to disk, then exec
  # the agent. Everything is base64 encoded so no shell quoting in the
  # scripts can break the entrypoint.
  entrypoint = ["sh", "-c", <<-EOT
    set -eu
    mkdir -p ${local.staging_dir}
    echo '${base64encode(file("${path.module}/scripts/sandbox-create.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-create.sh
    echo '${base64encode(file("${path.module}/scripts/sandbox-destroy.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-destroy.sh
    echo '${base64encode(file("${path.module}/scripts/sandbox-create-microvm.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-create-microvm.sh
    echo '${base64encode(file("${path.module}/scripts/sandbox-destroy-microvm.sh"))}' | base64 -d > ${local.staging_dir}/sandbox-destroy-microvm.sh
    echo '${base64encode(local.agent_init)}' | base64 -d > ${local.staging_dir}/init.sh
    chmod +x ${local.staging_dir}/sandbox-create.sh ${local.staging_dir}/sandbox-destroy.sh
    chmod +x ${local.staging_dir}/sandbox-create-microvm.sh ${local.staging_dir}/sandbox-destroy-microvm.sh

    # netns confinement needs the ip binary. This is best effort: if it is
    # missing the supervisor cleans up and degrades to advisory proxy mode
    # rather than running the agent unconfined.
    if ! command -v ip >/dev/null 2>&1; then
      sudo apt-get update -qq >/dev/null 2>&1 && sudo apt-get install -y -qq iproute2 >/dev/null 2>&1 || true
    fi

    exec sh ${local.staging_dir}/init.sh
  EOT
  ]

  env = local.agent_process_env

  # netns confinement creates a namespace and a veth pair.
  dynamic "capabilities" {
    for_each = local.confinement == "netns" ? [1] : []
    content {
      add = ["NET_ADMIN"]
    }
  }

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  # The Docker sandbox script drives Docker on the host. This grants the
  # workspace control of the Docker daemon, which is acceptable for a demo
  # and is a deliberate trust decision for anything else.
  dynamic "volumes" {
    for_each = local.sandbox_docker ? [1] : []
    content {
      container_path = "/var/run/docker.sock"
      host_path      = "/var/run/docker.sock"
      read_only      = false
    }
  }

  # The microVM backend needs hardware virtualization inside the workspace.
  dynamic "devices" {
    for_each = local.sandbox_microvm ? [1] : []
    content {
      host_path      = "/dev/kvm"
      container_path = "/dev/kvm"
      permissions    = "rwm"
    }
  }

  # coder/sandbox caches the msb and libkrunfw runtime here. Persisting it
  # across rebuilds avoids re-downloading, which matters because a confined
  # workspace cannot reach the download host unless the egress policy
  # allows it.
  dynamic "volumes" {
    for_each = local.sandbox_microvm ? [1] : []
    content {
      container_path = "/home/coder/.microsandbox"
      volume_name    = docker_volume.microsandbox_cache[0].name
      read_only      = false
    }
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
