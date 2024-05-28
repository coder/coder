terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.22.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.13.1"
    }
    google = {
      source  = "hashicorp/google"
      version = "4.46.0"
    }
  }
}

data "google_client_config" "provider" {}

data "google_container_cluster" "dev-4-2" {
  project  = "coder-dev-1"
  name     = "dev-4-2"
  location = "us-central1-a"
}

locals {
  namespace      = "colin-coder"
  workspace_name = lower("coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}")
  cpu            = 1
  memory         = "1Gi"
  gpu            = 1
}

provider "kubernetes" {
  host  = "https://${data.google_container_cluster.dev-4-2.endpoint}"
  token = data.google_client_config.provider.access_token
  cluster_ca_certificate = base64decode(
    data.google_container_cluster.dev-4-2.master_auth[0].cluster_ca_certificate,
  )
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
  startup_script = <<EOT
    #!/bin/bash
    # home folder can be empty, so copying default bash settings
    if [ ! -f ~/.profile ]; then
      cp /etc/skel/.profile $HOME
    fi
    if [ ! -f ~/.bashrc ]; then
      cp /etc/skel/.bashrc $HOME
    fi
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log
    code-server --auth none --port 13337 | tee code-server-install.log &
  EOT
}

resource "coder_app" "code-server" {
  agent_id  = coder_agent.main.id
  slug      = "code-server"
  icon      = "/icon/code.svg"
  url       = "http://localhost:13337?folder=/home/coder"
  subdomain = false
}

resource "kubernetes_config_map" "coder_workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
  }
}

resource "kubernetes_service_account" "coder_workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
  }
}
resource "kubernetes_secret" "coder_workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
    annotations = {
      "kubernetes.io/service-account.name"      = local.workspace_name
      "kubernetes.io/service-account.namespace" = local.namespace
    }
  }
  type = "kubernetes.io/service-account-token"
}

resource "kubernetes_role" "coder_workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
  }

  rule {
    api_groups     = ["*"]
    resources      = ["configmaps"]
    resource_names = [local.workspace_name]
    verbs          = ["*"]
  }
}

resource "kubernetes_role_binding" "coder_workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = local.workspace_name
  }
  subject {
    kind      = "ServiceAccount"
    name      = local.workspace_name
    namespace = local.namespace
  }
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  depends_on = [
    kubernetes_role.coder_workspace,
    kubernetes_role_binding.coder_workspace,
    kubernetes_service_account.coder_workspace,
    kubernetes_secret.coder_workspace,
    kubernetes_config_map.coder_workspace
  ]
  metadata {
    name      = local.workspace_name
    namespace = local.namespace
  }
  spec {
    service_account_name = local.workspace_name

    container {
      name    = "dev"
      image   = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      resources {
        limits = {
          cpu    = local.cpu
          memory = local.memory
        }
      }
    }
  }
}

resource "coder_metadata" "kubernetes_pod_main" {
  count       = data.coder_workspace.me.start_count
  resource_id = kubernetes_pod.main[0].id
  item {
    key   = "cpu"
    value = local.cpu
  }
  item {
    key   = "memory"
    value = local.memory
  }
  item {
    key   = "gpu"
    value = local.gpu
  }
}
