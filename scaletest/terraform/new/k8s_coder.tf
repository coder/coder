data "google_client_config" "default" {}

locals {
  coder_subdomain           = "${var.name}-primary-scaletest"
  coder_url                 = "http://${local.coder_subdomain}.${var.cloudflare_domain}"
  dnsNames                  = regex("http?://([^/]+)", local.coder_url)
  coder_europe_subdomain    = "${var.name}-europe-scaletest"
  coder_europe_url          = "http://${local.coder_europe_subdomain}.${var.cloudflare_domain}"
  coder_asia_subdomain      = "${var.name}-asia-scaletest"
  coder_asia_url            = "http://${local.coder_asia_subdomain}.${var.cloudflare_domain}"
  # coder_url                 = "https://${local.coder_subdomain}.${var.cloudflare_domain}"
  # dnsNames                  = regex("https?://([^/]+)", local.coder_url)
  coder_admin_email         = "admin@coder.com"
  coder_admin_full_name     = "Coder Admin"
  coder_admin_user          = "coder"
  coder_admin_password      = "SomeSecurePassword!"
  coder_helm_repo           = "https://helm.coder.com/v2"
  coder_helm_chart          = "coder"
  coder_namespace           = "coder-${var.name}"
  coder_release_name        = var.name
  provisionerd_helm_chart   = "coder-provisioner"
  provisionerd_release_name = "${var.name}-provisionerd"

}

resource "kubernetes_namespace" "coder_namespace" {
  provider = kubernetes.primary

  metadata {
    name = local.coder_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "kubernetes_namespace" "coder_europe" {
  provider = kubernetes.europe

  metadata {
    name = local.coder_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }
}

resource "random_password" "provisionerd_psk" {
  length = 26
}

resource "kubernetes_secret" "coder-db" {
  provider = kubernetes.primary

  type = "Opaque"
  metadata {
    name      = "coder-db-url"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    url = local.coder_db_url
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "kubernetes_secret" "provisionerd_psk" {
  provider = kubernetes.primary

  type = "Opaque"
  metadata {
    name      = "coder-provisioner-psk"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    psk = random_password.provisionerd_psk.result
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "kubernetes_secret" "provisionerd_psk_europe" {
  provider = kubernetes.europe

  type = "Opaque"
  metadata {
    name      = "coder-provisioner-psk"
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
  }
  data = {
    psk = random_password.provisionerd_psk.result
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

# OIDC secret needs to be manually provisioned for now.
data "kubernetes_secret" "coder_oidc" {
  provider = kubernetes.primary
  metadata {
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
    name      = "coder-oidc"
  }
}

data "kubernetes_secret" "coder_oidc_europe" {
  provider = kubernetes.europe
  metadata {
    namespace = kubernetes_namespace.coder_namespace.metadata.0.name
    name      = "coder-oidc"
  }
}

# resource "kubectl_manifest" "coder_certificate" {
#   provider = kubectl.primary

#   depends_on = [ helm_release.cert-manager ]
#   yaml_body = <<YAML
# apiVersion: cert-manager.io/v1
# kind: Certificate
# metadata:
#   name: ${var.name}
#   namespace: ${kubernetes_namespace.coder_namespace.metadata.0.name}
# spec:
#   secretName: ${var.name}-tls
#   dnsNames:
#   - ${local.coder_subdomain}.${var.cloudflare_domain}
#   issuerRef:
#     group: cert-manager.io
#     name: cloudflare-issuer
#     kind: ClusterIssuer
# YAML
# }

# data "kubernetes_secret" "coder_tls" {
#   provider = kubernetes.primary

#   metadata {
#     namespace = kubernetes_namespace.coder_namespace.metadata.0.name
#     name      = "${var.name}-tls"
#   }
#   depends_on = [kubectl_manifest.coder_certificate]
# }

resource "helm_release" "coder-chart" {
  provider = helm.primary

  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = kubernetes_namespace.coder_namespace.metadata.0.name
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["primary_coder"].name}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_ACCESS_URL"
      value: "${local.coder_url}"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PG_CONNECTION_URL"
      valueFrom:
        secretKeyRef:
          name: "${kubernetes_secret.coder-db.metadata.0.name}"
          key: url
    - name: "CODER_PPROF_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_AGENT_STATS"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_DB_METRICS"
      value: "true"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_EXPERIMENTS"
      value: "${var.coder_experiments}"
    - name: "CODER_DANGEROUS_DISABLE_RATE_LIMITS"
      value: "true"
    # Disabling built-in provisioner daemons
    - name: "CODER_PROVISIONER_DAEMONS"
      value: "0"
    - name: CODER_PROVISIONER_DAEMON_PSK
      valueFrom:
        secretKeyRef:
          key: psk
          name: "${kubernetes_secret.provisionerd_psk.metadata.0.name}"
    # Enable OIDC
    # - name: "CODER_OIDC_ISSUER_URL"
    #   valueFrom:
    #     secretKeyRef:
    #       key: issuer-url
    #       name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    # - name: "CODER_OIDC_EMAIL_DOMAIN"
    #   valueFrom:
    #     secretKeyRef:
    #       key: email-domain
    #       name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    # - name: "CODER_OIDC_CLIENT_ID"
    #   valueFrom:
    #     secretKeyRef:
    #       key: client-id
    #       name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    # - name: "CODER_OIDC_CLIENT_SECRET"
    #   valueFrom:
    #     secretKeyRef:
    #       key: client-secret
    #       name: "${data.kubernetes_secret.coder_oidc.metadata.0.name}"
    # Send OTEL traces to the cluster-local collector to sample 10%
    - name: "OTEL_EXPORTER_OTLP_ENDPOINT"
      value: "http://otel-collector.${kubernetes_namespace.coder_namespace.metadata.0.name}.svc.cluster.local:4317"
    - name: "OTEL_TRACES_SAMPLER"
      value: parentbased_traceidratio
    - name: "OTEL_TRACES_SAMPLER_ARG"
      value: "0.1"
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${var.coder_replicas}"
  resources:
    requests:
      cpu: "${var.coder_cpu_request}"
      memory: "${var.coder_mem_request}"
    limits:
      cpu: "${var.coder_cpu_limit}"
      memory: "${var.coder_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  service:
    enable: true
    sessionAffinity: None
    loadBalancerIP: "${google_compute_address.coder["primary"].address}"
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}


resource "kubernetes_secret" "proxy_token_europe" {
  provider = kubernetes.europe

  type = "Opaque"
  metadata {
    name      = "coder-proxy-token"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
  }
  data = {
    token = trimspace(data.local_file.europe_proxy_token.content)
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "helm_release" "coder_europe" {
  provider = helm.europe

  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = kubernetes_namespace.coder_europe.metadata.0.name
  values = [<<EOF
coder:
  workspaceProxy: true
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["europe_coder"].name}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: CODER_PRIMARY_ACCESS_URL
      value: "${local.coder_url}"
    - name: CODER_PROXY_SESSION_TOKEN
      valueFrom:
        secretKeyRef:
          key: token
          name: "${kubernetes_secret.proxy_token_europe.metadata.0.name}"
    - name: "CODER_ACCESS_URL"
      value: "${local.coder_europe_url}"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PPROF_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_AGENT_STATS"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_DB_METRICS"
      value: "true"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_EXPERIMENTS"
      value: "${var.coder_experiments}"
    - name: "CODER_DANGEROUS_DISABLE_RATE_LIMITS"
      value: "true"
    # Disabling built-in provisioner daemons
    - name: "CODER_PROVISIONER_DAEMONS"
      value: "0"
    - name: CODER_PROVISIONER_DAEMON_PSK
      valueFrom:
        secretKeyRef:
          key: psk
          name: "${kubernetes_secret.provisionerd_psk_europe.metadata.0.name}"
    # Enable OIDC
    # - name: "CODER_OIDC_ISSUER_URL"
    #   valueFrom:
    #     secretKeyRef:
    #       key: issuer-url
    #       name: "${data.kubernetes_secret.coder_oidc_europe.metadata.0.name}"
    # - name: "CODER_OIDC_EMAIL_DOMAIN"
    #   valueFrom:
    #     secretKeyRef:
    #       key: email-domain
    #       name: "${data.kubernetes_secret.coder_oidc_europe.metadata.0.name}"
    # - name: "CODER_OIDC_CLIENT_ID"
    #   valueFrom:
    #     secretKeyRef:
    #       key: client-id
    #       name: "${data.kubernetes_secret.coder_oidc_europe.metadata.0.name}"
    # - name: "CODER_OIDC_CLIENT_SECRET"
    #   valueFrom:
    #     secretKeyRef:
    #       key: client-secret
    #       name: "${data.kubernetes_secret.coder_oidc_europe.metadata.0.name}"
    # Send OTEL traces to the cluster-local collector to sample 10%
    - name: "OTEL_EXPORTER_OTLP_ENDPOINT"
      value: "http://otel-collector.${kubernetes_namespace.coder_europe.metadata.0.name}.svc.cluster.local:4317"
    - name: "OTEL_TRACES_SAMPLER"
      value: parentbased_traceidratio
    - name: "OTEL_TRACES_SAMPLER_ARG"
      value: "0.1"
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${var.coder_replicas}"
  resources:
    requests:
      cpu: "${var.coder_cpu_request}"
      memory: "${var.coder_mem_request}"
    limits:
      cpu: "${var.coder_cpu_limit}"
      memory: "${var.coder_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  service:
    enable: true
    sessionAffinity: None
    loadBalancerIP: "${google_compute_address.coder["europe"].address}"
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

resource "helm_release" "provisionerd-chart" {
  provider = helm.primary
  
  repository = local.coder_helm_repo
  chart      = local.provisionerd_helm_chart
  name       = local.provisionerd_release_name
  version    = var.provisionerd_chart_version
  namespace  = kubernetes_namespace.coder_namespace.metadata.0.name
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["primary_coder"].name}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_URL"
      value: "${local.coder_url}"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_CONFIG_DIR"
      value: "/tmp/config"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROVISIONERD_TAGS"
      value: "scope=organization"
  image:
    repo: ${var.provisionerd_image_repo}
    tag: ${var.provisionerd_image_tag}
  replicaCount: "${var.provisionerd_replicas}"
  resources:
    requests:
      cpu: "${var.provisionerd_cpu_request}"
      memory: "${var.provisionerd_mem_request}"
    limits:
      cpu: "${var.provisionerd_cpu_limit}"
      memory: "${var.provisionerd_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

resource "helm_release" "provisionerd_europe" {
  provider = helm.europe
  
  repository = local.coder_helm_repo
  chart      = local.provisionerd_helm_chart
  name       = local.provisionerd_release_name
  version    = var.provisionerd_chart_version
  namespace  = kubernetes_namespace.coder_europe.metadata.0.name
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${google_container_node_pool.node_pool["europe_coder"].name}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_URL"
      value: "${local.coder_url}"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_CONFIG_DIR"
      value: "/tmp/config"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROVISIONERD_TAGS"
      value: "scope=organization"
  image:
    repo: ${var.provisionerd_image_repo}
    tag: ${var.provisionerd_image_tag}
  replicaCount: "${var.provisionerd_replicas}"
  resources:
    requests:
      cpu: "${var.provisionerd_cpu_request}"
      memory: "${var.provisionerd_mem_request}"
    limits:
      cpu: "${var.provisionerd_cpu_limit}"
      memory: "${var.provisionerd_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

data "http" "coder_healthy" {
  url = local.coder_url
  // Wait up to 5 minutes for DNS to propogate
  retry {
    attempts = 30
    min_delay_ms = 10000
  }

  lifecycle {
    postcondition {
        condition = self.status_code == 200
        error_message = "${self.url} returned an unhealthy status code"
    }
  }

  depends_on = [ helm_release.coder-chart, cloudflare_record.coder ]
}

resource "null_resource" "proxy_tokens" {
  provisioner "local-exec" {
    interpreter = [ "/bin/bash", "-c" ]
    command = <<EOF
curl '${local.coder_url}/api/v2/users/first' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}","username":"${local.coder_admin_user}","name":"${local.coder_admin_full_name}","trial":false}' \
  --insecure --silent --output /dev/null

token=$(curl '${local.coder_url}/api/v2/users/login' \
  --data-raw $'{"email":"${local.coder_admin_email}","password":"${local.coder_admin_password}"}' \
  --insecure --silent | jq -r .session_token)

curl '${local.coder_url}/api/v2/licenses' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"license":"${var.coder_license}"}' \
  --insecure --silent --output /dev/null

europe_token=$(curl '${local.coder_url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"name":"europe"}' \
  --insecure --silent | jq -r .proxy_token)

asia_token=$(curl '${local.coder_url}/api/v2/workspaceproxies' \
  -H "Coder-Session-Token: $${token}" \
  --data-raw '{"name":"asia"}' \
  --insecure --silent | jq -r .proxy_token)

echo -n $${europe_token} > ${path.module}/europe_proxy_token
echo -n $${asia_token} > ${path.module}/asia_proxy_token
EOF
  }

  depends_on = [ data.http.coder_healthy ]
}

data "local_file" "europe_proxy_token" {
  filename = "${path.module}/europe_proxy_token"
  depends_on = [ null_resource.proxy_tokens ]
}

data "local_file" "asia_proxy_token" {
  filename = "${path.module}/asia_proxy_token"
  depends_on = [ null_resource.proxy_tokens ]
}

# data "external" "proxy_tokens" {
#   program = ["bash", "${path.module}/workspace_proxies.sh"]
#   query = {
#     coder_url = local.coder_url
#     coder_admin_email = local.coder_admin_email
#     coder_admin_password = local.coder_admin_password
#     coder_admin_user = local.coder_admin_user
#     coder_admin_full_name = local.coder_admin_full_name
#     coder_license = var.coder_license

#     status_code = data.http.coder_healthy.status_code
#   }

#   depends_on = [ data.http.coder_healthy ]
# }

