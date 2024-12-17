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
  values = [templatefile("${path.module}/coder_helm_values.tftpl", {
    workspace_proxy  = true,
    provisionerd     = false,
    primary_url      = local.deployments.primary.url,
    proxy_token      = kubernetes_secret.proxy_token_europe.metadata.0.name,
    db_secret        = null,
    ip_address       = google_compute_address.coder["europe"].address,
    provisionerd_psk = null,
    access_url       = local.deployments.europe.url,
    node_pool        = google_container_node_pool.node_pool["europe_coder"].name,
    release_name     = local.coder_release_name,
    experiments      = var.coder_experiments,
    image_repo       = var.coder_image_repo,
    image_tag        = var.coder_image_tag,
    replicas         = local.scenarios[var.scenario].coder.replicas,
    cpu_request      = local.scenarios[var.scenario].coder.cpu_request,
    mem_request      = local.scenarios[var.scenario].coder.mem_request,
    cpu_limit        = local.scenarios[var.scenario].coder.cpu_limit,
    mem_limit        = local.scenarios[var.scenario].coder.mem_limit,
    deployment       = "europe",
  })]

  depends_on = [null_resource.license]
}

resource "helm_release" "provisionerd_europe" {
  provider = helm.europe

  repository = local.coder_helm_repo
  chart      = local.provisionerd_helm_chart
  name       = local.provisionerd_release_name
  version    = var.provisionerd_chart_version
  namespace  = kubernetes_namespace.coder_europe.metadata.0.name
  values = [templatefile("${path.module}/coder_helm_values.tftpl", {
    workspace_proxy  = false,
    provisionerd     = true,
    primary_url      = null,
    proxy_token      = null,
    db_secret        = null,
    ip_address       = null,
    provisionerd_psk = kubernetes_secret.provisionerd_psk_europe.metadata.0.name,
    access_url       = local.deployments.primary.url,
    node_pool        = google_container_node_pool.node_pool["europe_coder"].name,
    release_name     = local.coder_release_name,
    experiments      = var.coder_experiments,
    image_repo       = var.coder_image_repo,
    image_tag        = var.coder_image_tag,
    replicas         = local.scenarios[var.scenario].provisionerd.replicas,
    cpu_request      = local.scenarios[var.scenario].provisionerd.cpu_request,
    mem_request      = local.scenarios[var.scenario].provisionerd.mem_request,
    cpu_limit        = local.scenarios[var.scenario].provisionerd.cpu_limit,
    mem_limit        = local.scenarios[var.scenario].provisionerd.mem_limit,
    deployment       = "europe",
  })]

  depends_on = [null_resource.license]
}
