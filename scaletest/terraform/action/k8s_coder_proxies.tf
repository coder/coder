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

resource "null_resource" "proxy_tokens" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
curl '${local.deployments.primary.url}/api/v2/users/first' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}","username":"${local.coder_admin_user}","name":"${local.coder_admin_full_name}","trial":false}' \
  --insecure --silent --output /dev/null

token=$(curl '${local.deployments.primary.url}/api/v2/users/login' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}"}' \
  --insecure --silent | jq -r .session_token)

curl '${local.deployments.primary.url}/api/v2/licenses' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"license":"${var.coder_license}"}' \
  --insecure --silent --output /dev/null

europe_token=$(curl '${local.deployments.primary.url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"name":"europe"}' \
  --insecure --silent | jq -r .proxy_token)

asia_token=$(curl '${local.deployments.primary.url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"name":"asia"}' \
  --insecure --silent | jq -r .proxy_token)

mkdir -p ${path.module}/.coderv2
echo -n $${europe_token} > ${path.module}/.coderv2/europe_proxy_token
echo -n $${asia_token} > ${path.module}/.coderv2/asia_proxy_token
EOF
  }

  depends_on = [data.http.coder_healthy]
}

data "local_file" "europe_proxy_token" {
  filename   = "${path.module}/.coderv2/europe_proxy_token"
  depends_on = [null_resource.proxy_tokens]
}

data "local_file" "asia_proxy_token" {
  filename   = "${path.module}/.coderv2/asia_proxy_token"
  depends_on = [null_resource.proxy_tokens]
}
