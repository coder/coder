resource "kubernetes_namespace" "coder_europe" {
  provider = kubernetes.europe

  metadata {
    name = local.coder_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }

  depends_on = [google_container_node_pool.node_pool["europe_misc"]]
}

resource "kubernetes_secret" "provisionerd_psk_europe" {
  provider = kubernetes.europe

  type = "Opaque"
  metadata {
    name      = "coder-provisioner-psk"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
  }
  data = {
    psk = random_password.provisionerd_psk.result
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "kubernetes_secret" "proxy_token_europe" {
  provider = kubernetes.europe

  type = "Opaque"
  metadata {
    name      = "coder-proxy-token"
    namespace = kubernetes_namespace.coder_europe.metadata.0.name
  }
  data = {
    token = coderd_workspace_proxy.europe.session_token
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
      value: "${local.deployments.primary.url}"
    - name: CODER_PROXY_SESSION_TOKEN
      valueFrom:
        secretKeyRef:
          key: token
          name: "${kubernetes_secret.proxy_token_europe.metadata.0.name}"
    - name: "CODER_ACCESS_URL"
      value: "${local.deployments.europe.url}"
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
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${local.scenarios[var.scenario].coder.replicas}"
  resources:
    requests:
      cpu: "${local.scenarios[var.scenario].coder.cpu_request}"
      memory: "${local.scenarios[var.scenario].coder.mem_request}"
    limits:
      cpu: "${local.scenarios[var.scenario].coder.cpu_limit}"
      memory: "${local.scenarios[var.scenario].coder.mem_limit}"
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
      value: "${local.deployments.primary.url}"
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
  replicaCount: "${local.scenarios[var.scenario].provisionerd.replicas}"
  resources:
    requests:
      cpu: "${local.scenarios[var.scenario].provisionerd.cpu_request}"
      memory: "${local.scenarios[var.scenario].provisionerd.mem_request}"
    limits:
      cpu: "${local.scenarios[var.scenario].provisionerd.cpu_limit}"
      memory: "${local.scenarios[var.scenario].provisionerd.mem_limit}"
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
