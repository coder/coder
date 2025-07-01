resource "local_file" "kubernetes_template" {
  filename = "${path.module}/.coderv2/templates/kubernetes/main.tf"
  content  = <<EOF
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 2.1.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
  }
}

provider "coder" {}

provider "kubernetes" {
  config_path = null # always use host
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os                     = "linux"
  arch                   = "amd64"
}

resource "coder_script" "websocat" {
  agent_id     = coder_agent.main.id
  display_name = "websocat"
  script       = <<EOF2
curl -sSL -o /tmp/websocat https://github.com/vi/websocat/releases/download/v1.12.0/websocat.x86_64-unknown-linux-musl
chmod +x /tmp/websocat

/tmp/websocat --exit-on-eof --binary ws-l:127.0.0.1:1234 mirror:
EOF2
  run_on_start = true
}

resource "coder_app" "wsecho" {
  agent_id     = coder_agent.main.id
  slug         = "wsec"
  display_name = "WebSocket Echo"
  url          = "http://localhost:1234"
  share        = "authenticated"
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-$${lower(data.coder_workspace_owner.me.name)}-$${lower(data.coder_workspace.me.name)}"
    namespace = "${local.coder_namespace}"
    labels = {
      "app.kubernetes.io/name"     = "coder-workspace"
      "app.kubernetes.io/instance" = "coder-workspace-$${lower(data.coder_workspace_owner.me.name)}-$${lower(data.coder_workspace.me.name)}"
    }
  }
  spec {
    security_context {
      run_as_user = "1000"
      fs_group    = "1000"
    }
    container {
      name              = "dev"
      image             = "${var.workspace_image}"
      image_pull_policy = "Always"
      command           = ["sh", "-c", coder_agent.main.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      resources {
        requests = {
          "cpu"    = "${local.scenarios[var.scenario].workspaces.cpu_request}"
          "memory" = "${local.scenarios[var.scenario].workspaces.mem_request}"
        }
        limits = {
          "cpu"    = "${local.scenarios[var.scenario].workspaces.cpu_limit}"
          "memory" = "${local.scenarios[var.scenario].workspaces.mem_limit}"
        }
      }
    }

    affinity {
      node_affinity {
        required_during_scheduling_ignored_during_execution {
          node_selector_term {
            match_expressions {
              key = "cloud.google.com/gke-nodepool"
              operator = "In"
              values = ["${google_container_node_pool.node_pool["primary_workspaces"].name}","${google_container_node_pool.node_pool["europe_workspaces"].name}","${google_container_node_pool.node_pool["asia_workspaces"].name}"]
            }
          }
        }
      }
    }
  }
}
EOF
}

resource "kubernetes_config_map" "template_primary" {
  provider = kubernetes.primary

  metadata {
    name      = "coder-template"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
  }

  data = {
    "main.tf" = local_file.kubernetes_template.content
  }
}

resource "kubernetes_job" "push_template_primary" {
  provider = kubernetes.primary

  metadata {
    name      = "${var.name}-push-template"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-push-template"
    }
  }
  spec {
    completions = 1
    template {
      metadata {}
      spec {
        affinity {
          node_affinity {
            required_during_scheduling_ignored_during_execution {
              node_selector_term {
                match_expressions {
                  key      = "cloud.google.com/gke-nodepool"
                  operator = "In"
                  values   = ["${google_container_node_pool.node_pool["primary_misc"].name}"]
                }
              }
            }
          }
        }
        container {
          name  = "cli"
          image = "${var.coder_image_repo}:${var.coder_image_tag}"
          command = [
            "/opt/coder",
            "--verbose",
            "--url=${local.deployments.primary.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "templates",
            "push",
            "--directory=/home/coder/template",
            "--provisioner-tag=scope=organization",
            "--provisioner-tag=deployment=primary",
            "--yes",
            "kubernetes-primary"
          ]
          volume_mount {
            name       = "coder-template"
            mount_path = "/home/coder/template/main.tf"
            sub_path   = "main.tf"
          }
        }
        volume {
          name = "coder-template"
          config_map {
            name = kubernetes_config_map.template_primary.metadata.0.name
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  depends_on = [helm_release.provisionerd_primary]
}

resource "kubernetes_config_map" "template_europe" {
  provider = kubernetes.europe

  metadata {
    name      = "coder-template"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
  }

  data = {
    "main.tf" = local_file.kubernetes_template.content
  }
}

resource "kubernetes_job" "push_template_europe" {
  provider = kubernetes.europe

  metadata {
    name      = "${var.name}-push-template"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-push-template"
    }
  }
  spec {
    completions = 1
    template {
      metadata {}
      spec {
        affinity {
          node_affinity {
            required_during_scheduling_ignored_during_execution {
              node_selector_term {
                match_expressions {
                  key      = "cloud.google.com/gke-nodepool"
                  operator = "In"
                  values   = ["${google_container_node_pool.node_pool["europe_misc"].name}"]
                }
              }
            }
          }
        }
        container {
          name  = "cli"
          image = "${var.coder_image_repo}:${var.coder_image_tag}"
          command = [
            "/opt/coder",
            "--verbose",
            "--url=${local.deployments.primary.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "templates",
            "push",
            "--directory=/home/coder/template",
            "--provisioner-tag=scope=organization",
            "--provisioner-tag=deployment=europe",
            "--yes",
            "kubernetes-europe"
          ]
          volume_mount {
            name       = "coder-template"
            mount_path = "/home/coder/template/main.tf"
            sub_path   = "main.tf"
          }
        }
        volume {
          name = "coder-template"
          config_map {
            name = kubernetes_config_map.template_europe.metadata.0.name
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  depends_on = [helm_release.provisionerd_europe]
}

resource "kubernetes_config_map" "template_asia" {
  provider = kubernetes.asia

  metadata {
    name      = "coder-template"
    namespace = kubernetes_namespace.coder_asia.metadata.0.name
  }

  data = {
    "main.tf" = local_file.kubernetes_template.content
  }
}

resource "kubernetes_job" "push_template_asia" {
  provider = kubernetes.asia

  metadata {
    name      = "${var.name}-push-template"
    namespace = kubernetes_namespace.coder_asia.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-push-template"
    }
  }
  spec {
    completions = 1
    template {
      metadata {}
      spec {
        affinity {
          node_affinity {
            required_during_scheduling_ignored_during_execution {
              node_selector_term {
                match_expressions {
                  key      = "cloud.google.com/gke-nodepool"
                  operator = "In"
                  values   = ["${google_container_node_pool.node_pool["asia_misc"].name}"]
                }
              }
            }
          }
        }
        container {
          name  = "cli"
          image = "${var.coder_image_repo}:${var.coder_image_tag}"
          command = [
            "/opt/coder",
            "--verbose",
            "--url=${local.deployments.primary.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "templates",
            "push",
            "--directory=/home/coder/template",
            "--provisioner-tag=scope=organization",
            "--provisioner-tag=deployment=asia",
            "--yes",
            "kubernetes-asia"
          ]
          volume_mount {
            name       = "coder-template"
            mount_path = "/home/coder/template/main.tf"
            sub_path   = "main.tf"
          }
        }
        volume {
          name = "coder-template"
          config_map {
            name = kubernetes_config_map.template_asia.metadata.0.name
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  depends_on = [helm_release.provisionerd_asia]
}
