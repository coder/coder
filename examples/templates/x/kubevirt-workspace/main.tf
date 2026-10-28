terraform {
  required_version = ">= 1.5.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.19.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "3.4.0"
    }
  }
}

provider "coder" {}

variable "kubeconfig_path" {
  type        = string
  description = "Kubeconfig on the provisioner, or null for in-cluster authentication."
  default     = null
}

provider "kubernetes" {
  config_path = var.kubeconfig_path
}

variable "namespace" {
  type        = string
  description = "Existing namespace dedicated to this Coder organization. KubeVirt and CDI must already be installed by a cluster administrator."
}

variable "storage_class" {
  type        = string
  description = "StorageClass for persistent VM root disks. Must support filesystem ReadWriteOnce volumes."
}

variable "image_url" {
  type        = string
  description = "Administrator-approved, versioned HTTPS URL for an Ubuntu 24.04 amd64 cloud image. Mirror and verify the image before production use."
  validation {
    condition     = startswith(var.image_url, "https://")
    error_message = "Use an HTTPS cloud image URL."
  }
}

variable "image_sha256" {
  type        = string
  description = "SHA-256 from the verified publisher checksum file for image_url. CDI rejects an image with different content."
  validation {
    condition     = can(regex("^[a-f0-9]{64}$", var.image_sha256))
    error_message = "Use a lowercase, 64-character SHA-256 digest."
  }
}

variable "ssh_public_key" {
  type        = string
  description = "Optional first-boot ed25519 recovery key for the lab. It persists on the root disk; clearing this variable does not revoke it. No SSH Service is created."
  default     = ""
  validation {
    condition     = var.ssh_public_key == "" || can(regex("^ssh-ed25519 [A-Za-z0-9+/=]+( [^\\r\\n]*)?$", var.ssh_public_key))
    error_message = "Use a single-line ed25519 public key, or leave empty."
  }
}

variable "guest_network_cidr" {
  type        = string
  description = "Private guest-side masquerade subnet. Must not overlap networks the guest needs to reach."
  default     = "192.168.251.0/24"
  validation {
    condition     = can(cidrnetmask(var.guest_network_cidr))
    error_message = "Use an IPv4 CIDR."
  }
}

variable "cpu_model" {
  type        = string
  description = "KubeVirt CPU model. Use host-model with KVM. For the emulated lab, select a model advertised by the node, such as Nehalem."
  default     = "host-model"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU cores"
  type         = "number"
  default      = "2"
  mutable      = true
  validation {
    min = 1
    max = 8
  }
}

data "coder_parameter" "memory" {
  name         = "memory"
  display_name = "Memory (GiB)"
  type         = "number"
  default      = "4"
  mutable      = true
  validation {
    min = 2
    max = 32
  }
}

data "coder_parameter" "disk_size" {
  name         = "disk_size"
  display_name = "Persistent root disk (GiB)"
  type         = "number"
  default      = "24"
  mutable      = false
  validation {
    min = 16
    max = 256
  }
}

locals {
  name = "coder-${data.coder_workspace.me.id}"
  labels = {
    "app.kubernetes.io/part-of" = "coder-kubevirt-prototype"
    "com.coder.workspace.id"    = data.coder_workspace.me.id
    "com.coder.user.id"         = data.coder_workspace_owner.me.id
  }
}

resource "coder_agent" "main" {
  count = data.coder_workspace.me.start_count
  arch  = "amd64"
  os    = "linux"

  startup_script_behavior = "blocking"
  connection_timeout      = 1800
  startup_script          = <<-EOT
    #!/bin/sh
    set -eu
    docker info >/dev/null
    docker compose version
  EOT
}

# Keep the DataVolume outside the VM's dataVolumeTemplates and start_count.
# Deleting compute on stop must not delete the root disk or Docker state.
resource "kubernetes_manifest" "root" {
  manifest = {
    apiVersion = "cdi.kubevirt.io/v1beta1"
    kind       = "DataVolume"
    metadata = {
      name      = "${local.name}-root"
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      source = {
        http = {
          url      = var.image_url
          checksum = "sha256:${var.image_sha256}"
        }
      }
      storage = {
        storageClassName = var.storage_class
        volumeMode       = "Filesystem"
        accessModes      = ["ReadWriteOnce"]
        resources = {
          requests = { storage = "${data.coder_parameter.disk_size.value}Gi" }
        }
      }
    }
  }
}

resource "kubernetes_secret_v1" "cloudinit" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "${local.name}-cloudinit"
    namespace = var.namespace
    labels    = local.labels
  }
  data = {
    userdata = templatefile("${path.module}/cloud-config.yaml.tftpl", {
      hostname        = local.name
      authorized_keys = jsonencode(compact([var.ssh_public_key]))
    })
  }
}

# KubeVirt snapshots this Secret into a read-only disk at VMI creation.
# Never put these credentials into cloud-init or the persistent root disk.
resource "kubernetes_secret_v1" "agent" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "${local.name}-agent"
    namespace = var.namespace
    labels    = local.labels
  }
  data = {
    "agent.env"    = "CODER_AGENT_TOKEN=${coder_agent.main[0].token}\n"
    "init.sh"      = coder_agent.main[0].init_script
    "bootstrap.sh" = file("${path.module}/bootstrap-agent.sh")
  }
}

resource "kubernetes_manifest" "vm" {
  count = data.coder_workspace.me.start_count
  manifest = {
    apiVersion = "kubevirt.io/v1"
    kind       = "VirtualMachine"
    metadata = {
      name      = local.name
      namespace = var.namespace
      labels    = local.labels
    }
    spec = {
      runStrategy = "Always"
      template = {
        metadata = {
          labels = local.labels
          # KubeVirt adds this to untyped template metadata. Declare its shape
          # so the Kubernetes provider can preserve it in Terraform state.
          annotations = { "kubevirt.io/pci-topology-version" = "v3" }
        }
        spec = {
          terminationGracePeriodSeconds = 120
          domain = {
            cpu = {
              cores = tonumber(data.coder_parameter.cpu.value)
              model = var.cpu_model
            }
            # Stable identity avoids re-running first-boot package installation.
            firmware = {
              uuid   = data.coder_workspace.me.id
              serial = data.coder_workspace.me.id
            }
            resources = {
              requests = { memory = "${data.coder_parameter.memory.value}Gi" }
            }
            devices = {
              disks = [
                { name = "root", disk = { bus = "virtio" } },
                { name = "cloudinit", disk = { bus = "virtio" } },
                {
                  name   = "agent"
                  serial = "CODERAGENT"
                  disk   = { bus = "virtio", readonly = true }
                },
              ]
              interfaces = [{ name = "default", masquerade = {} }]
            }
          }
          networks = [{ name = "default", pod = { vmNetworkCIDR = var.guest_network_cidr } }]
          volumes = [
            {
              name       = "root"
              dataVolume = { name = kubernetes_manifest.root.manifest.metadata.name }
            },
            {
              name = "cloudinit"
              cloudInitNoCloud = {
                secretRef = { name = kubernetes_secret_v1.cloudinit[0].metadata[0].name }
                networkData = yamlencode({
                  version = 2
                  ethernets = {
                    default = {
                      match = { name = "e*" }
                      dhcp4 = true
                    }
                  }
                })
              }
            },
            {
              name = "agent"
              secret = {
                secretName  = kubernetes_secret_v1.agent[0].metadata[0].name
                volumeLabel = "CODERAGENT"
              }
            },
          ]
        }
      }
    }
  }

  # Record the VM in state promptly. Coder waits for the connected agent and
  # its blocking startup script; the live tests also check KubeVirt readiness.
  timeouts {
    create = "5m"
    update = "5m"
    delete = "10m"
  }
}
