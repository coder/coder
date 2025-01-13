data "google_client_config" "default" {}

locals {
  coder_admin_email         = "admin@coder.com"
  coder_admin_full_name     = "Coder Admin"
  coder_admin_user          = "coder"
  coder_admin_password      = "SomeSecurePassword!"
  coder_helm_repo           = "https://helm.coder.com/v2"
  coder_helm_chart          = "coder"
  coder_namespace           = "coder"
  coder_release_name        = "${var.name}-coder"
  provisionerd_helm_chart   = "coder-provisioner"
  provisionerd_release_name = "${var.name}-provisionerd"

}

resource "random_password" "provisionerd_psk" {
  length = 26
}

resource "kubernetes_namespace" "coder_primary" {
  provider = kubernetes.primary

  metadata {
    name = local.coder_namespace
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_default_service_account]
  }

  depends_on = [google_container_node_pool.node_pool["primary_misc"]]
}

resource "kubernetes_secret" "coder_db" {
  provider = kubernetes.primary

  type = "Opaque"
  metadata {
    name      = "coder-db-url"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
  }
  data = {
    url = local.coder_db_url
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "kubernetes_secret" "provisionerd_psk_primary" {
  provider = kubernetes.primary

  type = "Opaque"
  metadata {
    name      = "coder-provisioner-psk"
    namespace = kubernetes_namespace.coder_primary.metadata.0.name
  }
  data = {
    psk = random_password.provisionerd_psk.result
  }
  lifecycle {
    ignore_changes = [timeouts, wait_for_service_account_token]
  }
}

resource "helm_release" "coder_primary" {
  provider = helm.primary

  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = kubernetes_namespace.coder_primary.metadata.0.name
  values = [templatefile("${path.module}/coder_helm_values.tftpl", {
    workspace_proxy  = false,
    provisionerd     = false,
    primary_url      = null,
    proxy_token      = null,
    db_secret        = kubernetes_secret.coder_db.metadata.0.name,
    ip_address       = google_compute_address.coder["primary"].address,
    provisionerd_psk = kubernetes_secret.provisionerd_psk_primary.metadata.0.name,
    access_url       = local.deployments.primary.url,
    node_pool        = google_container_node_pool.node_pool["primary_coder"].name,
    release_name     = local.coder_release_name,
    experiments      = var.coder_experiments,
    image_repo       = var.coder_image_repo,
    image_tag        = var.coder_image_tag,
    replicas         = local.scenarios[var.scenario].coder.replicas,
    cpu_request      = local.scenarios[var.scenario].coder.cpu_request,
    mem_request      = local.scenarios[var.scenario].coder.mem_request,
    cpu_limit        = local.scenarios[var.scenario].coder.cpu_limit,
    mem_limit        = local.scenarios[var.scenario].coder.mem_limit,
    deployment       = "primary",
  })]
}

resource "helm_release" "provisionerd_primary" {
  provider = helm.primary

  repository = local.coder_helm_repo
  chart      = local.provisionerd_helm_chart
  name       = local.provisionerd_release_name
  version    = var.provisionerd_chart_version
  namespace  = kubernetes_namespace.coder_primary.metadata.0.name
  values = [templatefile("${path.module}/coder_helm_values.tftpl", {
    workspace_proxy  = false,
    provisionerd     = true,
    primary_url      = null,
    proxy_token      = null,
    db_secret        = null,
    ip_address       = null,
    provisionerd_psk = kubernetes_secret.provisionerd_psk_primary.metadata.0.name,
    access_url       = local.deployments.primary.url,
    node_pool        = google_container_node_pool.node_pool["primary_coder"].name,
    release_name     = local.coder_release_name,
    experiments      = var.coder_experiments,
    image_repo       = var.coder_image_repo,
    image_tag        = var.coder_image_tag,
    replicas         = local.scenarios[var.scenario].provisionerd.replicas,
    cpu_request      = local.scenarios[var.scenario].provisionerd.cpu_request,
    mem_request      = local.scenarios[var.scenario].provisionerd.mem_request,
    cpu_limit        = local.scenarios[var.scenario].provisionerd.cpu_limit,
    mem_limit        = local.scenarios[var.scenario].provisionerd.mem_limit,
    deployment       = "primary",
  })]

  depends_on = [null_resource.license]
}
