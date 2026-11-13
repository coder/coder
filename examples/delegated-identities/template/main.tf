# A workspace for one agent: a pod that Terraform makes, kept ready ahead of
# time as a prebuilt workspace, whose daemon proves which pod it is with the
# kubelet's token. Nothing in the pod depends on who owns the workspace, so
# claiming one replaces nothing.
#
# The template knows nothing about delegated identities. Whoever owns the
# workspace is who the agent in it is to Coder: a person, or a delegated
# identity the workspace was created on behalf of.
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

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kube_context == "" ? null : var.kube_context
}

variable "kubeconfig_path" {
  type        = string
  description = "Kubeconfig the provisioner uses to reach the cluster."
  default     = "~/.kube/config"
}

variable "kube_context" {
  type        = string
  description = "Kubeconfig context. Empty uses the current context."
  default     = ""
}

variable "namespace" {
  type        = string
  description = "Namespace of the pods."
  default     = "coder-sandbox-workloads"
}

variable "users_config_map" {
  type        = string
  description = "A ConfigMap with passwd and group files that name the pod's user. The daemon needs the name for terminals."
  default     = "coder-workload-users"
}

variable "pool_size" {
  type        = number
  description = "How many workspaces are kept ready."
  default     = 3
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

# The pool: this many workspaces of the template are built ahead of time
# and wait for an owner.
data "coder_workspace_preset" "agent" {
  name    = "agent"
  default = true
  prebuilds {
    instances = var.pool_size
  }
}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  # The daemon is whoever the cluster says its pod is. It is given no token.
  auth = "kubernetes-pod-identity"
}

# What runs in the workspace reaches Coder as the workspace's owner. The
# values travel in the daemon's manifest, not in the pod, so a claimed
# workspace gets its new owner's without a new pod.
resource "coder_env" "coder_url" {
  agent_id = coder_agent.main.id
  name     = "CODER_URL"
  value    = data.coder_workspace.me.access_url
}

resource "coder_env" "coder_session_token" {
  agent_id = coder_agent.main.id
  name     = "CODER_SESSION_TOKEN"
  value    = data.coder_workspace_owner.me.session_token
}

resource "kubernetes_pod_v1" "workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
    namespace = var.namespace
  }
  spec {
    # The service account can do nothing in the cluster, and its ordinary
    # token is not mounted.
    service_account_name             = "coder-workload"
    automount_service_account_token  = false
    enable_service_links             = false
    termination_grace_period_seconds = 5
    security_context {
      run_as_non_root = true
      run_as_user     = 1000
      run_as_group    = 1000
      seccomp_profile {
        type = "RuntimeDefault"
      }
    }
    container {
      name  = "workspace"
      image = "alpine:3.22"
      # The script downloads the daemon from Coder and starts it. It holds
      # no credential.
      command = ["sh", "-c", coder_agent.main.init_script]
      env {
        name  = "CODER_AGENT_DEVCONTAINERS_ENABLE"
        value = "false"
      }
      env {
        name  = "HOME"
        value = "/home/coder"
      }
      security_context {
        allow_privilege_escalation = false
        run_as_non_root            = true
        capabilities {
          drop = ["ALL"]
        }
      }
      # An agent that waits for its model uses next to nothing. Ask for
      # little, so that many fit on a node, and allow much, so that one
      # that works is not held back.
      resources {
        requests = {
          cpu    = "50m"
          memory = "64Mi"
        }
        limits = {
          cpu    = "2"
          memory = "1Gi"
        }
      }
      volume_mount {
        name       = "coder-identity"
        mount_path = "/var/run/secrets/coder.com/identity"
        read_only  = true
      }
      volume_mount {
        name       = "users"
        mount_path = "/etc/passwd"
        sub_path   = "passwd"
        read_only  = true
      }
      volume_mount {
        name       = "users"
        mount_path = "/etc/group"
        sub_path   = "group"
        read_only  = true
      }
      volume_mount {
        name       = "home"
        mount_path = "/home/coder"
      }
      volume_mount {
        name       = "tmp"
        mount_path = "/tmp"
      }
    }
    # The kubelet obtains this token for the pod and keeps it fresh. It is
    # only good for telling Coder which pod this is.
    volume {
      name = "coder-identity"
      projected {
        sources {
          service_account_token {
            audience           = data.coder_workspace.me.access_url
            expiration_seconds = 600
            path               = "token"
          }
        }
      }
    }
    volume {
      name = "users"
      config_map {
        name = var.users_config_map
      }
    }
    volume {
      name = "home"
      empty_dir {}
    }
    volume {
      name = "tmp"
      empty_dir {}
    }
  }
}

# The daemon in the pod is the daemon of this workspace because the cluster
# says it runs in this pod.
resource "coder_agent_instance" "pod" {
  count       = data.coder_workspace.me.start_count
  agent_id    = coder_agent.main.id
  instance_id = "k8s-pod:${kubernetes_pod_v1.workspace[0].metadata[0].uid}"
}
