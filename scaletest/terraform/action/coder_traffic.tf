locals {
  wait_baseline_duration = "60s"
  workspace_traffic_job_timeout = "420s"
  workspace_traffic_duration    = "300s"
  bytes_per_tick                = 1024
  tick_interval                 = "100ms"
}

resource "time_sleep" "wait_baseline" {
  depends_on = [
    kubernetes_job.create_workspaces_primary,
    kubernetes_job.create_workspaces_europe,
    kubernetes_job.create_workspaces_asia,
  ]

  create_duration = local.wait_baseline_duration
}

resource "kubernetes_job" "workspace_traffic_primary" {
  provider = kubernetes.primary

  metadata {
    name      = "${var.name}-workspace-traffic"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic"
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
            "exp",
            "scaletest",
            "workspace-traffic",
            "--template=kubernetes-primary",
            "--concurrency=0",
            "--bytes-per-tick=${local.bytes_per_tick}",
            "--tick-interval=${local.tick_interval}",
            "--scaletest-prometheus-wait=60s",
          ]
          env {
            name  = "CODER_SCALETEST_JOB_TIMEOUT"
            value = local.workspace_traffic_duration
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.workspace_traffic_job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}

resource "kubernetes_job" "workspace_traffic_europe" {
  provider = kubernetes.europe

  metadata {
    name      = "${var.name}-workspace-traffic"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic"
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
            "--url=${local.deployments.europe.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "exp",
            "scaletest",
            "workspace-traffic",
            "--template=kubernetes-europe",
            "--concurrency=0",
            "--bytes-per-tick=${local.bytes_per_tick}",
            "--tick-interval=${local.tick_interval}",
            "--scaletest-prometheus-wait=60s",
          ]
          env {
            name  = "CODER_SCALETEST_JOB_TIMEOUT"
            value = local.workspace_traffic_duration
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.workspace_traffic_job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}

resource "kubernetes_job" "workspace_traffic_asia" {
  provider = kubernetes.asia

  metadata {
    name      = "${var.name}-workspace-traffic"
    namespace = kubernetes_namespace.coder_asia.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic"
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
            "--url=${local.deployments.asia.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "exp",
            "scaletest",
            "workspace-traffic",
            "--template=kubernetes-asia",
            "--concurrency=0",
            "--bytes-per-tick=${local.bytes_per_tick}",
            "--tick-interval=${local.tick_interval}",
            "--scaletest-prometheus-wait=60s",
          ]
          env {
            name  = "CODER_SCALETEST_JOB_TIMEOUT"
            value = local.workspace_traffic_duration
          }
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.workspace_traffic_job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}
