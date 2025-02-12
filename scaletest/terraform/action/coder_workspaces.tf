locals {
  create_workspace_timeout = "30m"
}

resource "kubernetes_job" "create_workspaces_primary" {
  provider = kubernetes.primary

  metadata {
    name      = "${var.name}-create-workspaces"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-create-workspaces"
    }
  }
  spec {
    completions   = 1
    backoff_limit = 0
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
            "exp",
            "scaletest",
            "create-workspaces",
            "--count=${local.scenarios[var.scenario].workspaces.count_per_deployment}",
            "--template=kubernetes-primary",
            "--concurrency=${local.scenarios[var.scenario].provisionerd.replicas}",
            "--no-cleanup"
          ]
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.create_workspace_timeout
  }

  depends_on = [kubernetes_job.push_template_primary]
}

resource "kubernetes_job" "create_workspaces_europe" {
  provider = kubernetes.europe

  metadata {
    name      = "${var.name}-create-workspaces"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-create-workspaces"
    }
  }
  spec {
    completions   = 1
    backoff_limit = 0
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
            "exp",
            "scaletest",
            "create-workspaces",
            "--count=${local.scenarios[var.scenario].workspaces.count_per_deployment}",
            "--template=kubernetes-europe",
            "--concurrency=${local.scenarios[var.scenario].provisionerd.replicas}",
            "--no-cleanup"
          ]
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.create_workspace_timeout
  }

  depends_on = [kubernetes_job.push_template_europe]
}

resource "kubernetes_job" "create_workspaces_asia" {
  provider = kubernetes.asia

  metadata {
    name      = "${var.name}-create-workspaces"
    namespace = kubernetes_namespace.coder_asia.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-create-workspaces"
    }
  }
  spec {
    completions   = 1
    backoff_limit = 0
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
            "exp",
            "scaletest",
            "create-workspaces",
            "--count=${local.scenarios[var.scenario].workspaces.count_per_deployment}",
            "--template=kubernetes-asia",
            "--concurrency=${local.scenarios[var.scenario].provisionerd.replicas}",
            "--no-cleanup"
          ]
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.create_workspace_timeout
  }

  depends_on = [kubernetes_job.push_template_asia]
}
