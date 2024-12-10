data "http" "coder_healthy" {
  url = local.deployments.primary.url
  // Wait up to 5 minutes for DNS to propagate
  retry {
    attempts     = 30
    min_delay_ms = 10000
  }

  lifecycle {
    postcondition {
      condition     = self.status_code == 200
      error_message = "${self.url} returned an unhealthy status code"
    }
  }

  depends_on = [helm_release.coder_primary, cloudflare_record.coder["primary"]]
}

resource "null_resource" "api_key" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
curl '${local.deployments.primary.url}/api/v2/users/first' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}","username":"${local.coder_admin_user}","name":"${local.coder_admin_full_name}","trial":false}' \
  --insecure --silent --output /dev/null

session_token=$(curl '${local.deployments.primary.url}/api/v2/users/login' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}"}' \
  --insecure --silent | jq -r .session_token)

api_key=$(curl '${local.deployments.primary.url}/users/me/keys/tokens' \
  -H "Coder-Session-Token: $${session_token}" \
  --data-raw '{"token_name":"terraform","scope":"all"}' \
  --insecure --silent | jq -r .key)

mkdir -p ${path.module}/.coderv2
echo -n $${api_key} > ${path.module}/.coderv2/api_key
EOF
  }

  depends_on = [data.http.coder_healthy]
}

data "local_file" "api_key" {
  filename   = "${path.module}/.coderv2/api_key"
  depends_on = [null_resource.api_key]
}

resource "coderd_license" "license" {
  license = var.coder_license
  lifecycle {
    create_before_destroy = true
  }
}

resource "coderd_workspace_proxy" "europe" {
  name         = "europe"
  display_name = "Europe"
  icon         = "/emojis/1f950.png"

  depends_on = [coderd_license.license]
}

resource "coderd_workspace_proxy" "asia" {
  name         = "asia"
  display_name = "Asia"
  icon         = "/emojis/1f35b.png"

  depends_on = [coderd_license.license]
}

resource "local_file" "kubernetes_template" {
  filename = "${path.module}/.coderv2/templates/kubernetes/main.tf"
  content  = <<EOF
    terraform {
      required_providers {
        coder = {
          source  = "coder/coder"
          version = "~> 0.23.0"
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
              "cpu"    = "${local.scenarios[var.scenario].workspace.cpu_request}"
              "memory" = "${local.scenarios[var.scenario].workspace.mem_request}"
            }
            limits = {
              "cpu"    = "${local.scenarios[var.scenario].workspace.cpu_limit}"
              "memory" = "${local.scenarios[var.scenario].workspace.mem_limit}"
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

resource "coderd_template" "kubernetes" {
  name = "kubernetes"
  versions = [{
    directory = "${path.module}/.coderv2/templates/kubernetes"
    active    = true
  }]
}
