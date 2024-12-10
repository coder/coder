# resource "kubernetes_pod" "create_workspaces" {
#   provider = kubernetes.primary

#   metadata {
#     name      = "${var.name}-create-workspaces"
#     namespace = kubernetes_namespace.coder_primary.metadata.0.name
#     labels = {
#       "app.kubernetes.io/name" = "${var.name}-create-workspaces"
#     }
#   }
#   spec {
#     affinity {
#       node_affinity {
#         required_during_scheduling_ignored_during_execution {
#           node_selector_term {
#             match_expressions {
#               key      = "cloud.google.com/gke-nodepool"
#               operator = "In"
#               values   = ["${google_container_node_pool.node_pool["primary_misc"].name}"]
#             }
#           }
#         }
#       }
#     }
#     container {
#       name    = "cli"
#       image   = "${var.coder_image_repo}:${var.coder_image_tag}"
#       command = ["/opt/coder --verbose --url=${local.deployments.primary.url} --token=${trimspace(data.local_file.api_key.content)} exp scaletest create-workspaces --count ${var.workspace_count} --template=kubernetes --concurrency ${var.workspace_create_concurrency} --no-cleanup"]
#     }
#     restart_policy = "Never"
#   }

#   depends_on = [ coderd_template.kubernetes ]
# }

# resource "time_sleep" "wait_for_baseline" {
#   depends_on = [kubernetes_pod.create_workspaces]

#   create_duration = "600s"
# }

# resource "kubernetes_pod" "workspace_traffic_primary" {
#   provider = kubernetes.primary

#   metadata {
#     name      = "${var.name}-traffic"
#     namespace = kubernetes_namespace.coder.metadata.0.name
#     labels = {
#       "app.kubernetes.io/name" = "${var.name}-traffic"
#     }
#   }
#   spec {
#     affinity {
#       node_affinity {
#         required_during_scheduling_ignored_during_execution {
#           node_selector_term {
#             match_expressions {
#               key      = "cloud.google.com/gke-nodepool"
#               operator = "In"
#               values   = ["${google_container_node_pool.node_pool["primary_misc"].name}"]
#             }
#           }
#         }
#       }
#     }
#     container {
#       name    = "cli"
#       image   = "${var.coder_image_repo}:${var.coder_image_tag}"
#       command = ["/opt/coder --verbose --url=${local.deployments.primary.url} --token=${trimspace(local_file.api_key.content)} exp scaletest workspace-traffic --concurrency=0 --bytes-per-tick=${var.traffic_bytes_per_tick} --tick-interval=${var.traffic_tick_interval} --scaletest-prometheus-wait=60s"]

#       env {
#         name  = "CODER_URL"
#         value = local.deployments.primary.url
#       }
#       env {
#         name  = "CODER_TOKEN"
#         value = trimspace(local_file.api_key.content)
#       }
#       env {
#         name  = "CODER_SCALETEST_PROMETHEUS_ADDRESS"
#         value = "0.0.0.0:21112"
#       }
#       env {
#         name  = "CODER_SCALETEST_JOB_TIMEOUT"
#         value = "30m"
#       }
#       port {
#         container_port = 21112
#         name           = "prometheus-http"
#         protocol       = "TCP"
#       }
#     }
#     restart_policy = "Never"
#   }

#   depends_on = [time_sleep.wait_for_baseline]
# }
