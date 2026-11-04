terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 2.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.9"
    }
  }
}

variable "docker_socket" {
  description = "Docker socket URI on the provisioner. The daemon must support privileged Linux containers and cgroup v2."
  type        = string
  default     = "unix:///var/run/docker.sock"
}

variable "coder_url" {
  description = "Optional agent download and connection URL. Leave unset to use the deployment URL. Loopback hosts are mapped to the Docker host."
  type        = string
  default     = null
}

provider "coder" {
  url = var.coder_url
}

provider "docker" {
  host = var.docker_socket
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  # Leave capacity for the agent, Docker, image preparation, and page cache.
  host_cpus           = 8
  host_memory_mib     = 16384
  workload_cpus       = 4
  workload_memory_mib = 8192
  labels = {
    "coder.workspace_id" = data.coder_workspace.me.id
    "coder.owner_id"     = data.coder_workspace_owner.me.id
  }
}

resource "coder_agent" "host" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"

  # Bootstrap also sets this before launching the agent. Agent env alone is
  # delivered too late to initialize the workload supervisor.
  env = {
    CODER_AGENT_WORKLOAD_CONFIG = "/var/lib/coder-workload-host/config.json"
    WORKLOAD_CONTROL_PLANE_URL  = replace(coalesce(var.coder_url, data.coder_workspace.me.access_url), "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
  }

  metadata {
    key          = "workloads_total"
    display_name = "Total admitted workloads"
    script       = "bash /opt/workloads/workload-metadata.sh total"
    interval     = 10
    timeout      = 10
  }
  metadata {
    key          = "workloads_running"
    display_name = "Running workloads"
    script       = "bash /opt/workloads/workload-metadata.sh running"
    interval     = 10
    timeout      = 10
  }
}

resource "docker_image" "host" {
  name = "coder-workload-host:${data.coder_workspace.me.id}"
  # Keep credentials, Terraform state, and the project recipe out of the host
  # image build context. Coder directory uploads omit hidden ignore files.
  build {
    context = "${path.module}/host"
    # The provider resolves Dockerfile paths from Terraform's working directory,
    # not the build context. Never use the project Dockerfile for the host.
    dockerfile = "${path.module}/host/Dockerfile"
  }
  triggers = {
    source = sha256(join("", [for name in ["Dockerfile", "start-host.sh", "prepare-host.sh"] : filesha256("${path.module}/host/${name}")]))
  }
}

# Both volumes survive stop/start. Never discard uncertain execution records
# to make a host start again. Workspace deletion removes these volumes.
resource "docker_volume" "data" {
  for_each = toset(["docker", "state"])
  name     = "coder-workloads-${data.coder_workspace.me.id}-${each.key}"
  lifecycle {
    ignore_changes = all
  }
  dynamic "labels" {
    for_each = local.labels
    content {
      label = labels.key
      value = labels.value
    }
  }
}

resource "docker_container" "host" {
  count = data.coder_workspace.me.start_count
  name  = "coder-workloads-${data.coder_workspace.me.id}"
  image = docker_image.host.image_id

  # This is an administrator-owned service host, not an untrusted user's
  # interactive workspace. The outer Docker socket is never mounted inside.
  privileged            = true
  init                  = true
  cgroupns_mode         = "private"
  cpu_period            = 100000
  cpu_quota             = local.host_cpus * 100000
  memory                = local.host_memory_mib
  memory_swap           = local.host_memory_mib
  restart               = "no"
  destroy_grace_seconds = 30

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.host.token}",
    "WORKLOAD_WORKSPACE_ID=${data.coder_workspace.me.id}",
    "WORKLOAD_CPUS=${local.workload_cpus}",
    "WORKLOAD_MEMORY_MIB=${local.workload_memory_mib}",
  ]

  upload {
    file        = "/opt/workloads/project/Dockerfile"
    content     = file("${path.module}/Dockerfile")
    permissions = "0600"
  }
  upload {
    file        = "/opt/workloads/workload-metadata.sh"
    content     = file("${path.module}/workload-metadata.sh")
    permissions = "0700"
  }
  upload {
    file        = "/opt/workloads/agent-init.sh"
    content     = replace(coder_agent.host.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
    permissions = "0700"
  }
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/var/lib/docker"
    volume_name    = docker_volume.data["docker"].name
  }
  volumes {
    container_path = "/var/lib/coder-workload-host"
    volume_name    = docker_volume.data["state"].name
  }
  dynamic "labels" {
    for_each = local.labels
    content {
      label = labels.key
      value = labels.value
    }
  }
}
