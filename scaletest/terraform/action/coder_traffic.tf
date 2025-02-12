locals {
  wait_baseline_duration = "5m"
  bytes_per_tick         = 1024
  tick_interval          = "100ms"

  traffic_types = {
    ssh = {
      duration    = "30m"
      job_timeout = "35m"
      flags = [
        "--ssh",
      ]
    }
    webterminal = {
      duration    = "25m"
      job_timeout = "30m"
      flags       = []
    }
    app = {
      duration    = "20m"
      job_timeout = "25m"
      flags = [
        "--app=wsec",
      ]
    }
  }
}

resource "time_sleep" "wait_baseline" {
  depends_on = [
    kubernetes_job.create_workspaces_primary,
    kubernetes_job.create_workspaces_europe,
    kubernetes_job.create_workspaces_asia,
    helm_release.prometheus_chart_primary,
    helm_release.prometheus_chart_europe,
    helm_release.prometheus_chart_asia,
  ]

  create_duration = local.wait_baseline_duration
}

resource "kubernetes_job" "workspace_traffic_primary" {
  provider = kubernetes.primary

  for_each = local.traffic_types
  metadata {
    name      = "${var.name}-workspace-traffic-${each.key}"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic-${each.key}"
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
          command = concat([
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
            "--scaletest-prometheus-wait=30s",
            "--job-timeout=${local.traffic_types[each.key].duration}",
          ], local.traffic_types[each.key].flags)
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.traffic_types[each.key].job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}

resource "kubernetes_job" "workspace_traffic_europe" {
  provider = kubernetes.europe

  for_each = local.traffic_types
  metadata {
    name      = "${var.name}-workspace-traffic-${each.key}"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic-${each.key}"
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
          command = concat([
            "/opt/coder",
            "--verbose",
            "--url=${local.deployments.primary.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "exp",
            "scaletest",
            "workspace-traffic",
            "--template=kubernetes-europe",
            "--concurrency=0",
            "--bytes-per-tick=${local.bytes_per_tick}",
            "--tick-interval=${local.tick_interval}",
            "--scaletest-prometheus-wait=30s",
            "--job-timeout=${local.traffic_types[each.key].duration}",
            "--workspace-proxy-url=${local.deployments.europe.url}",
          ], local.traffic_types[each.key].flags)
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.traffic_types[each.key].job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}

resource "kubernetes_job" "workspace_traffic_asia" {
  provider = kubernetes.asia

  for_each = local.traffic_types
  metadata {
    name      = "${var.name}-workspace-traffic-${each.key}"
    namespace = kubernetes_namespace.coder_asia.metadata.0.name
    labels = {
      "app.kubernetes.io/name" = "${var.name}-workspace-traffic-${each.key}"
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
          command = concat([
            "/opt/coder",
            "--verbose",
            "--url=${local.deployments.primary.url}",
            "--token=${trimspace(data.local_file.api_key.content)}",
            "exp",
            "scaletest",
            "workspace-traffic",
            "--template=kubernetes-asia",
            "--concurrency=0",
            "--bytes-per-tick=${local.bytes_per_tick}",
            "--tick-interval=${local.tick_interval}",
            "--scaletest-prometheus-wait=30s",
            "--job-timeout=${local.traffic_types[each.key].duration}",
            "--workspace-proxy-url=${local.deployments.asia.url}",
          ], local.traffic_types[each.key].flags)
        }
        restart_policy = "Never"
      }
    }
  }
  wait_for_completion = true

  timeouts {
    create = local.traffic_types[each.key].job_timeout
  }

  depends_on = [time_sleep.wait_baseline]
}
