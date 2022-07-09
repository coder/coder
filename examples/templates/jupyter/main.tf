terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.2"
    }
  }
}

data "google_client_config" "provider" {}

data "google_container_cluster" "my_cluster" {
  project  = "coder-dogfood"
  name     = "master"
  location = "us-central1-a"
}

provider "kubernetes" {
  host  = "https://${data.google_container_cluster.my_cluster.endpoint}"
  token = data.google_client_config.provider.access_token
  cluster_ca_certificate = base64decode(
  data.google_container_cluster.my_cluster.master_auth[0].cluster_ca_certificate,
  )
}

data "coder_workspace" "me" {}
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
  startup_script = <<-EOF
jupyter lab --ServerApp.base_url=/@${data.coder_workspace.me.owner}/${data.coder_workspace.me.name}/apps/Jupyter/ --ServerApp.token='' --ip='*'
EOF
}

resource "coder_app" "Jupyter" {
  agent_id = coder_agent.coder.id
  url = "http://localhost:8888/@${data.coder_workspace.me.owner}/${data.coder_workspace.me.name}/apps/Jupyter"
  icon = "/icon/jupyter.svg"
}

resource "kubernetes_deployment" "coder" {
  count = data.coder_workspace.me.start_count
  metadata {
    name = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}"
    labels = {
      "coder.workspace.owner" : data.coder_workspace.me.owner
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        "coder.workspace.id" : data.coder_workspace.me.id
      }
    }
    template {
      metadata {
        labels = {
          "coder.workspace.id" = data.coder_workspace.me.id
        }
      }
      spec {
        hostname                         = lower(data.coder_workspace.me.name)
        termination_grace_period_seconds = 1
        container {
          name    = "dev"
          image   = "codercom/enterprise-jupyter:ubuntu"
          command = ["sh", "-c", coder_agent.coder.init_script]
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.coder.token
          }
          security_context {
            run_as_user  = 1000
            run_as_group = 1000
          }
        }
        security_context {
          fs_group = 1000
        }
      }
    }
  }
  timeouts {
    create = "5m"
    delete = "5m"
    update = "5m"
  }
}
