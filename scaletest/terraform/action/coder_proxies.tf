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
set -e

curl '${local.deployments.primary.url}/api/v2/users/first' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}","username":"${local.coder_admin_user}","name":"${local.coder_admin_full_name}","trial":false}' \
  --insecure --silent --output /dev/null

session_token=$(curl '${local.deployments.primary.url}/api/v2/users/login' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}"}' \
  --insecure --silent | jq -r .session_token)

echo -n $${session_token} > ${path.module}/.coderv2/session_token

api_key=$(curl '${local.deployments.primary.url}/api/v2/users/me/keys/tokens' \
  -H "Coder-Session-Token: $${session_token}" \
  --data-raw '{"token_name":"terraform","scope":"all"}' \
  --insecure --silent | jq -r .key)

echo -n $${api_key} > ${path.module}/.coderv2/api_key
EOF
  }

  depends_on = [data.http.coder_healthy]
}

data "local_file" "api_key" {
  filename   = "${path.module}/.coderv2/api_key"
  depends_on = [null_resource.api_key]
}

resource "null_resource" "license" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
curl '${local.deployments.primary.url}/api/v2/licenses' \
  -H "Coder-Session-Token: ${trimspace(data.local_file.api_key.content)}" \
  --data-raw '{"license":"${var.coder_license}"}' \
  --insecure --silent --output /dev/null
EOF
  }
}

resource "null_resource" "europe_proxy_token" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
curl '${local.deployments.primary.url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: ${trimspace(data.local_file.api_key.content)}" \
  --data-raw '{"name":"europe","display_name":"Europe","icon":"/emojis/1f950.png"}' \
  --insecure --silent \
  | jq -r .proxy_token > ${path.module}/.coderv2/europe_proxy_token
EOF
  }

  depends_on = [null_resource.license]
}

data "local_file" "europe_proxy_token" {
  filename   = "${path.module}/.coderv2/europe_proxy_token"
  depends_on = [null_resource.europe_proxy_token]
}

resource "null_resource" "asia_proxy_token" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
curl '${local.deployments.primary.url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: ${trimspace(data.local_file.api_key.content)}" \
  --data-raw '{"name":"asia","display_name":"Asia","icon":"/emojis/1f35b.png"}' \
  --insecure --silent \
  | jq -r .proxy_token > ${path.module}/.coderv2/asia_proxy_token
EOF
  }

  depends_on = [null_resource.license]
}

data "local_file" "asia_proxy_token" {
  filename   = "${path.module}/.coderv2/asia_proxy_token"
  depends_on = [null_resource.asia_proxy_token]
}
