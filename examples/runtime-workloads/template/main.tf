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

provider "coder" {}

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
  description = "Namespace for the workspace's pod."
  default     = "coder-hosted-workloads"
}

variable "image" {
  type        = string
  description = "Image for the human's container."
  default     = "alpine:3.22"
}

variable "sandbox_image" {
  type        = string
  description = "Image for the container that runs workloads. It must provide dockerd."
  default     = "docker:27-dind"
}

variable "workload_image" {
  type        = string
  description = "Image for workload sandboxes that do not name one."
  default     = "alpine:3.22"
}

variable "workload_allowed_images" {
  type        = string
  description = "Comma-separated patterns of other images a workload may ask for. Empty allows only the default."
  default     = ""
}

variable "workload_network" {
  type        = string
  description = "What a sandbox can reach unless the workload asks otherwise: \"none\" or \"outbound\". A workload reaches Coder in both."
  default     = "none"
}

variable "workload_allowed_networks" {
  type        = string
  description = "Comma-separated networks a workload may ask for besides the default. \"none\" is always allowed."
  default     = "outbound"
}

variable "workload_allowed_mounts" {
  type        = string
  description = "Comma-separated directories that may be shared with workloads. Empty shares nothing."
  default     = "/projects"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

locals {
  name = "coder-${data.coder_workspace.me.id}"

  # What Docker needs to run containers inside a pod that is itself in a
  # user namespace. They are capabilities over the pod's own namespaces, not
  # over the node.
  sandbox_capabilities = [
    "SYS_ADMIN", "NET_ADMIN", "NET_RAW", "SYS_CHROOT", "SETUID", "SETGID",
    "CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "MKNOD", "SETPCAP",
    "SETFCAP", "SYS_PTRACE", "SYS_RESOURCE", "NET_BIND_SERVICE", "AUDIT_WRITE",
  ]
}

# The human's daemon. It runs in an ordinary container and cannot run
# workloads.
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  dir  = "/root"
}

# The daemon that runs workloads, in sandboxes. It has a container of its
# own, so a workload that escapes its sandbox does not land next to the
# human's files or the human daemon's credential.
resource "coder_agent" "sandbox" {
  os   = "linux"
  arch = "amd64"

  # This daemon only vouches for workloads. It cannot fetch the owner's Git
  # SSH key or external auth tokens.
  api_key_scope = "no_user_data"

  display_apps {
    vscode                 = false
    vscode_insiders        = false
    web_terminal           = false
    ssh_helper             = false
    port_forwarding_helper = false
  }
}

# The typed Deployment resource cannot set hostUsers, procMount or the
# AppArmor profile, so the Deployment is written as a manifest.
resource "kubernetes_manifest" "workspace" {
  count = data.coder_workspace.me.start_count

  manifest = {
    apiVersion = "apps/v1"
    kind       = "Deployment"
    metadata = {
      name      = local.name
      namespace = var.namespace
      labels = {
        "coder.com/workspace-id" = data.coder_workspace.me.id
      }
    }
    spec = {
      replicas = 1
      selector = {
        matchLabels = {
          "app.kubernetes.io/instance" = local.name
        }
      }
      template = {
        metadata = {
          labels = {
            "app.kubernetes.io/instance" = local.name
            "coder.com/workspace-id"     = data.coder_workspace.me.id
          }
        }
        spec = {
          # The pod has a user namespace of its own: root in the pod is an
          # unprivileged user on the node. Nothing here is privileged.
          hostUsers                    = false
          automountServiceAccountToken = false

          containers = [
            {
              name    = "dev"
              image   = var.image
              command = ["sh", "-c", coder_agent.main.init_script]
              env = [
                { name = "CODER_AGENT_TOKEN", value = coder_agent.main.token },
              ]
              # What the human puts here can be shared with workloads. The
              # rest of this container, the home directory included, cannot.
              volumeMounts = [
                { name = "projects", mountPath = "/projects" },
              ]
              resources = {
                requests = { cpu = "100m", memory = "256Mi" }
                limits   = { memory = "1Gi" }
              }
            },
            {
              name  = "sandbox"
              image = var.sandbox_image
              # Docker runs next to the daemon, in the same container,
              # because the two share files: each sandbox reaches the daemon
              # through a socket in a directory that Docker mounts into it.
              #
              # Docker runs in rootless mode, because cgroups are not
              # delegated to a pod in a user namespace. It listens on its
              # unix socket only: a TCP listener would be reachable from
              # sandboxes. --icc=false keeps sandboxes from reaching each
              # other.
              command = ["sh", "-c", <<-EOT
                mkdir -p /run/user/0
                XDG_RUNTIME_DIR=/run/user/0 dockerd --rootless --host=unix:///var/run/docker.sock --icc=false >/var/log/dockerd.log 2>&1 &
                until docker version >/dev/null 2>&1; do sleep 1; done
                docker pull --quiet "$CODER_AGENT_WORKLOAD_IMAGE" || true
                ${coder_agent.sandbox.init_script}
              EOT
              ]
              securityContext = {
                privileged = false
                # Containers inside the pod need their own /proc.
                procMount       = "Unmasked"
                appArmorProfile = { type = "Unconfined" }
                seccompProfile  = { type = "Unconfined" }
                capabilities    = { add = local.sandbox_capabilities }
              }
              # An optional variable is left out when it is empty: the
              # provider cannot apply an empty string in a manifest.
              env = concat(
                [
                  { name = "CODER_AGENT_TOKEN", value = coder_agent.sandbox.token },
                  # Makes this daemon run workloads, each in a container.
                  { name = "CODER_AGENT_WORKLOAD_RUNTIME", value = "docker" },
                  # What a workload may be is decided here, by the template.
                  { name = "CODER_AGENT_WORKLOAD_IMAGE", value = var.workload_image },
                  { name = "CODER_AGENT_WORKLOAD_NETWORK", value = var.workload_network },
                  # Sandboxes are not devcontainers.
                  { name = "CODER_AGENT_DEVCONTAINERS_ENABLE", value = "false" },
                  # The two daemons share the pod's network namespace.
                  { name = "CODER_AGENT_DEBUG_ADDRESS", value = "127.0.0.1:2123" },
                  { name = "CODER_AGENT_PPROF_ADDRESS", value = "127.0.0.1:6061" },
                  { name = "CODER_AGENT_PROMETHEUS_ADDRESS", value = "127.0.0.1:2122" },
                ],
                var.workload_allowed_images == "" ? [] : [
                  { name = "CODER_AGENT_WORKLOAD_ALLOWED_IMAGES", value = var.workload_allowed_images },
                ],
                var.workload_allowed_networks == "" ? [] : [
                  { name = "CODER_AGENT_WORKLOAD_ALLOWED_NETWORKS", value = var.workload_allowed_networks },
                ],
                var.workload_allowed_mounts == "" ? [] : [
                  { name = "CODER_AGENT_WORKLOAD_ALLOWED_MOUNTS", value = var.workload_allowed_mounts },
                ],
              )
              volumeMounts = [
                { name = "docker", mountPath = "/var/lib/docker" },
                { name = "projects", mountPath = "/projects" },
              ]
              resources = {
                requests = { cpu = "250m", memory = "512Mi" }
                limits   = { memory = "2Gi" }
              }
            },
          ]
          volumes = [
            { name = "docker", emptyDir = {} },
            # Kept simple here: it is emptied when the workspace stops. Use a
            # persistent volume for real work.
            { name = "projects", emptyDir = {} },
          ]
        }
      }
    }
  }
}
