terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 2.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.6"
    }
  }
}

variable "docker_socket" {
  type        = string
  default     = ""
  description = "Docker socket used by the provisioner."
}

variable "claude_auth_file" {
  type        = string
  description = "Absolute Docker-host path to an owner-only Claude authentication JSON file. Mounted read-only, never uploaded in the template."
  validation {
    condition     = startswith(var.claude_auth_file, "/")
    error_message = "Use an absolute path on the Docker host."
  }
}

provider "docker" {
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "prompt" {
  name         = "prompt"
  display_name = "Initial prompt"
  description  = "The task to submit once when this workspace is first created (maximum 32,000 characters). This parameter is stored by Coder; do not include secrets."
  type         = "string"
  form_type    = "textarea"
  default      = "Summarize what you can help with in this workspace. Do not use tools yet."
  mutable      = false
  order        = 1
  validation {
    regex = "\\S"
    error = "Enter a prompt containing non-whitespace text."
  }
}

data "coder_parameter" "repository" {
  name         = "repository"
  display_name = "Repository URL"
  description  = "Optional public HTTPS Git repository. Leave empty to use a new project directory."
  type         = "string"
  default      = ""
  mutable      = false
  order        = 2
  validation {
    regex = "^$|^https://\\S+$"
    error = "Use a public HTTPS repository URL, or leave this empty."
  }
}

data "coder_parameter" "model" {
  name         = "model"
  display_name = "Claude model"
  description  = "Model for the background session."
  type         = "string"
  default      = "haiku"
  mutable      = false
  order        = 3
  option {
    name  = "Haiku"
    value = "haiku"
  }
  option {
    name  = "Sonnet"
    value = "sonnet"
  }
  option {
    name  = "Opus"
    value = "opus"
  }
}

data "coder_parameter" "trust_project" {
  name         = "trust_project"
  display_name = "Trust this project"
  description  = "Allow Claude to load this project's instructions and configuration at startup. Required for unattended launch. Tool permissions are not bypassed."
  type         = "bool"
  default      = "false"
  mutable      = false
  order        = 4
}

locals {
  image_hash = substr(sha256(join("", [for filename in sort(fileset("${path.module}/image", "**")) : filesha256("${path.module}/image/${filename}")])), 0, 16)
}

resource "docker_image" "workspace" {
  name         = "coder-native-agent:${local.image_hash}"
  keep_locally = true
  build {
    context = "${path.module}/image"
  }
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
  dir  = "/home/coder/project"
  env = {
    PATH = "/home/coder/.local/bin:/usr/local/bin:/usr/bin:/bin"
    # Agent environment values undergo variable expansion before startup.
    CODER_NATIVE_JOB_PROMPT_BASE64 = base64encode(data.coder_parameter.prompt.value)
    CODER_NATIVE_JOB_REPO_BASE64   = base64encode(data.coder_parameter.repository.value)
    CODER_NATIVE_JOB_MODEL         = data.coder_parameter.model.value
    CODER_NATIVE_JOB_TRUST         = data.coder_parameter.trust_project.value
    GIT_AUTHOR_NAME                = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL               = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME             = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL            = data.coder_workspace_owner.me.email
  }

  metadata {
    key          = "native_job"
    display_name = "Native job"
    script       = "node /opt/native-agent/job-status.mjs"
    interval     = 10
    timeout      = 2
  }
}

resource "coder_script" "job" {
  agent_id           = coder_agent.main.id
  display_name       = "Start native background job"
  run_on_start       = true
  start_blocks_login = true
  script             = "#!/bin/sh\nset -eu\nexec node /opt/native-agent/startup.mjs\n"
}

resource "docker_volume" "home" {
  name = "coder-native-agent-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
}

resource "docker_container" "workspace" {
  count         = data.coder_workspace.me.start_count
  name          = "coder-native-agent-${data.coder_workspace.me.id}"
  hostname      = data.coder_workspace.me.name
  image         = docker_image.workspace.image_id
  init          = true
  memory        = 4096
  security_opts = ["no-new-privileges:true"]
  capabilities {
    drop = ["ALL"]
  }
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home.name
  }
  volumes {
    container_path = "/run/secrets/claude-auth.json"
    host_path      = var.claude_auth_file
    read_only      = true
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
}
