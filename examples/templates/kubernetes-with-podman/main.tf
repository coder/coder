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
  config_path = "~/.kube/config"
}

data "coder_workspace" "me" {}

data "coder_parameter" "os" {
  name         = "os"
  display_name = "Operating system"
  description  = "The operating system to use for your workspace."
  default      = "ubuntu"
  option {
    name  = "Ubuntu"
    value = "ubuntu"
    icon  = "/icon/ubuntu.svg"
  }
  option {
    name  = "Fedora"
    value = "fedora"
    icon  = "/icon/fedora.svg"
  }
}

data "coder_parameter" "cpu" {
  name         = "cpu"
  display_name = "CPU"
  description  = "The number of CPU cores"
  default      = "2"
  option {
    name  = "2 Cores"
    value = "2"
  }
  option {
    name  = "4 Cores"
    value = "4"
  }
  option {
    name  = "6 Cores"
    value = "6"
  }
  option {
    name  = "8 Cores"
    value = "8"
  }
}

data "coder_parameter" "memory" {
  name         = "memory"
  display_name = "Memory"
  description  = "The amount of memory (in GB)"
  default      = "2"
  option {
    name  = "2 GB"
    value = "2"
  }
  option {
    name  = "4 GB"
    value = "4"
  }
  option {
    name  = "6 GB"
    value = "6"
  }
  option {
    name  = "8 GB"
    value = "8"
  }
}

resource "coder_agent" "dev" {
  os             = "linux"
  arch           = "amd64"
  dir            = "/home/podman"
  startup_script = <<EOF
    #!/bin/sh

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.11.0
    /tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &

    # Run once to avoid unnecessary warning: "/" is not a shared mount
    podman ps
  EOF

}

# code-server
resource "coder_app" "code-server" {
  agent_id     = coder_agent.dev.id
  display_name = "code-server"
  slug         = "code-server"
  icon         = "/icon/code.svg"
  url          = "http://localhost:13337"
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  depends_on = [
    kubernetes_persistent_volume_claim.home-directory
  ]
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
    namespace = "default"
    annotations = {
      # Disables apparmor, required for Debian- and Ubuntu-derived systems
      "container.apparmor.security.beta.kubernetes.io/dev" = "unconfined"
    }
  }
  spec {
    security_context {
      # Runs as the "podman" user
      run_as_user = 1000
      fs_group    = 1000
    }
    container {
      name = "dev"
      # We recommend building your own from our reference: see ./images directory
      image             = "ghcr.io/coder/podman:${data.coder_parameter.os.value}"
      image_pull_policy = "Always"
      command           = ["/bin/bash", "-c", coder_agent.dev.init_script]
      security_context {
        # Runs as the "podman" user
        run_as_user = "1000"
      }
      resources {
        requests = {
          "cpu"    = "250m"
          "memory" = "500Mi"
        }
        limits = {
          # Acquire a FUSE device, powered by smarter-device-manager
          "github.com/fuse" : 1
          cpu    = "${data.coder_parameter.cpu.value}"
          memory = "${data.coder_parameter.memory.value}Gi"
        }

      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }
      volume_mount {
        mount_path = "/home/podman"
        name       = "home-directory"
      }
    }
    volume {
      name = "home-directory"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.home-directory.metadata.0.name
      }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "home-directory" {
  metadata {
    name      = "coder-pvc-${data.coder_workspace.me.id}"
    namespace = "default"
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "10Gi"
      }
    }
  }
}
