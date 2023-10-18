terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
  }
}

data "coder_provisioner" "me" {
}

provider "coder" {
}

variable "use_kubeconfig" {
  type        = bool
  description = <<-EOF
  Use host kubeconfig? (true/false)

  Set this to false if the Coder host is itself running as a Pod on the same
  Kubernetes cluster as you are deploying workspaces to.

  Set this to true if the Coder host is running outside the Kubernetes cluster
  for workspaces.  A valid "~/.kube/config" must be present on the Coder host.
  EOF
  default     = false
}

variable "namespace" {
  type        = string
  description = "The Kubernetes namespace to create workspaces in (must exist prior to creating workspaces)"
}

provider "kubernetes" {
  # Authenticate via ~/.kube/config or a Coder-specific ServiceAccount, depending on admin preferences
  config_path = var.use_kubeconfig == true ? "~/.kube/config" : null
}


data "coder_workspace" "me" {
}

resource "coder_agent" "main" {
  arch                   = data.coder_provisioner.me.arch
  os                     = "linux"
  startup_script_timeout = 180
  startup_script         = <<-EOT
    set -e

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.11.0
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &
  EOT
  dir                    = "/workspaces"

  # These environment variables allow you to make Git commits right away after creating a
  # workspace. Note that they take precedence over configuration defined in ~/.gitconfig!
  # You can remove this block if you'd prefer to configure Git manually or using
  # dotfiles. (see docs/dotfiles.md)
  env = {
    GIT_AUTHOR_NAME     = "${data.coder_workspace.me.owner}"
    GIT_COMMITTER_NAME  = "${data.coder_workspace.me.owner}"
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace.me.owner_email}"
    GIT_COMMITTER_EMAIL = "${data.coder_workspace.me.owner_email}"
  }

}

resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  slug         = "code-server"
  display_name = "code-server"
  url          = "http://localhost:13337/?folder=/workspaces"
  icon         = "/icon/code.svg"
  subdomain    = false
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 5
    threshold = 6
  }
}

resource "kubernetes_persistent_volume_claim" "workspaces" {
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
    namespace = var.namespace
    labels = {
      "coder.owner"                      = data.coder_workspace.me.owner
      "coder.owner_id"                   = data.coder_workspace.me.owner_id
      "coder.workspace_id"               = data.coder_workspace.me.id
      "coder.workspace_name_at_creation" = data.coder_workspace.me.name
    }
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "10Gi" // adjust as needed
      }
    }
  }
  lifecycle {
    ignore_changes = all
  }
}

data "coder_parameter" "repo" {
  name         = "repo"
  display_name = "Repository (auto)"
  order        = 1
  description  = "Select a repository to automatically clone and start working with a devcontainer."
  mutable      = true
  option {
    name        = "vercel/next.js"
    description = "The React Framework"
    value       = "https://github.com/vercel/next.js"
  }
  option {
    name        = "home-assistant/core"
    description = "🏡 Open source home automation that puts local control and privacy first."
    value       = "https://github.com/home-assistant/core"
  }
  option {
    name        = "discourse/discourse"
    description = "A platform for community discussion. Free, open, simple."
    value       = "https://github.com/discourse/discourse"
  }
  option {
    name        = "denoland/deno"
    description = "A modern runtime for JavaScript and TypeScript."
    value       = "https://github.com/denoland/deno"
  }
  option {
    name        = "microsoft/vscode"
    icon        = "/icon/code.svg"
    description = "Code editing. Redefined."
    value       = "https://github.com/microsoft/vscode"
  }
  option {
    name        = "Custom"
    icon        = "/emojis/1f5c3.png"
    description = "Specify a custom repo URL below"
    value       = "custom"
  }
}

data "coder_parameter" "custom_repo_url" {
  name         = "custom_repo"
  display_name = "Repository URL (custom)"
  order        = 2
  default      = ""
  description  = "Optionally enter a custom repository URL, see [awesome-devcontainers](https://github.com/manekinekko/awesome-devcontainers)."
  mutable      = true
}

resource "kubernetes_deployment" "workspace" {
  metadata {
    name      = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
    namespace = var.namespace
    labels = {
      "coder.owner"          = data.coder_workspace.me.owner
      "coder.owner_id"       = data.coder_workspace.me.owner_id
      "coder.workspace_id"   = data.coder_workspace.me.id
      "coder.workspace_name" = data.coder_workspace.me.name
    }
  }
  spec {
    replicas = data.coder_workspace.me.start_count
    selector {
      match_labels = {
        "coder.workspace_id" = data.coder_workspace.me.id
      }
    }
    strategy {
      type = "Recreate"
    }
    template {
      metadata {
        labels = {
          "coder.workspace_id" = data.coder_workspace.me.id
        }
      }
      spec {
        container {
          name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
          # Find the latest version here:
          # https://github.com/coder/envbuilder/tags
          image = "ghcr.io/coder/envbuilder:0.2.1"
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          env {
            name  = "CODER_AGENT_URL"
            value = replace(data.coder_workspace.me.access_url, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
          }
          env {
            name  = "GIT_URL"
            value = data.coder_parameter.repo.value == "custom" ? data.coder_parameter.custom_repo_url.value : data.coder_parameter.repo.value
          }
          env {
            name  = "INIT_SCRIPT"
            value = replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")
          }
          env {
            name  = "FALLBACK_IMAGE"
            value = "codercom/enterprise-base:ubuntu"
          }
          volume_mount {
            name       = "workspaces"
            mount_path = "/workspaces"
          }
        }
        volume {
          name = "workspaces"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.workspaces.metadata.0.name
          }
        }
      }
    }
  }
}
