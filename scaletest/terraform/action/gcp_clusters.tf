data "google_compute_default_service_account" "default" {
  project    = var.project_id
  depends_on = [google_project_service.api["compute.googleapis.com"]]
}

locals {
  deployments = {
    primary = {
      subdomain           = "primary.${var.name}"
      wildcard_subdomain  = "*.primary.${var.name}"
      url                 = "https://primary.${var.name}.${var.cloudflare_domain}"
      wildcard_access_url = "*.primary.${var.name}.${var.cloudflare_domain}"
      region              = "us-east1"
      zone                = "us-east1-c"
      subnet              = "scaletest"
    }
    europe = {
      subdomain           = "europe.${var.name}"
      wildcard_subdomain  = "*.europe.${var.name}"
      url                 = "https://europe.${var.name}.${var.cloudflare_domain}"
      wildcard_access_url = "*.europe.${var.name}.${var.cloudflare_domain}"
      region              = "europe-west1"
      zone                = "europe-west1-b"
      subnet              = "scaletest"
    }
    asia = {
      subdomain           = "asia.${var.name}"
      wildcard_subdomain  = "*.asia.${var.name}"
      url                 = "https://asia.${var.name}.${var.cloudflare_domain}"
      wildcard_access_url = "*.asia.${var.name}.${var.cloudflare_domain}"
      region              = "asia-southeast1"
      zone                = "asia-southeast1-a"
      subnet              = "scaletest"
    }
  }
  node_pools = {
    primary_coder = {
      name    = "coder"
      cluster = "primary"
    }
    primary_workspaces = {
      name    = "workspaces"
      cluster = "primary"
    }
    primary_misc = {
      name    = "misc"
      cluster = "primary"
    }
    europe_coder = {
      name    = "coder"
      cluster = "europe"
    }
    europe_workspaces = {
      name    = "workspaces"
      cluster = "europe"
    }
    europe_misc = {
      name    = "misc"
      cluster = "europe"
    }
    asia_coder = {
      name    = "coder"
      cluster = "asia"
    }
    asia_workspaces = {
      name    = "workspaces"
      cluster = "asia"
    }
    asia_misc = {
      name    = "misc"
      cluster = "asia"
    }
  }
}

resource "google_container_cluster" "cluster" {
  for_each                  = local.deployments
  name                      = "${var.name}-${each.key}"
  location                  = each.value.zone
  project                   = var.project_id
  network                   = google_compute_network.network.name
  subnetwork                = google_compute_subnetwork.subnetwork[each.key].name
  networking_mode           = "VPC_NATIVE"
  default_max_pods_per_node = 256
  ip_allocation_policy { # Required with networking_mode=VPC_NATIVE
    cluster_secondary_range_name  = local.secondary_ip_range_k8s_pods
    services_secondary_range_name = local.secondary_ip_range_k8s_services
  }
  release_channel {
    # Setting release channel as STABLE can cause unexpected cluster upgrades.
    channel = "UNSPECIFIED"
  }
  initial_node_count       = 1
  remove_default_node_pool = true

  network_policy {
    enabled = true
  }
  depends_on = [
    google_project_service.api["container.googleapis.com"]
  ]
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
    managed_prometheus {
      enabled = false
    }
  }
  workload_identity_config {
    workload_pool = "${data.google_project.project.project_id}.svc.id.goog"
  }

  lifecycle {
    ignore_changes = [
      maintenance_policy,
      release_channel,
      remove_default_node_pool
    ]
  }
}

resource "google_container_node_pool" "node_pool" {
  for_each   = local.node_pools
  name       = each.value.name
  location   = local.deployments[each.value.cluster].zone
  project    = var.project_id
  cluster    = google_container_cluster.cluster[each.value.cluster].name
  node_count = local.scenarios[var.scenario][each.value.name].nodepool_size
  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/trace.append",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/servicecontrol",
    ]
    disk_size_gb    = 100
    machine_type    = local.scenarios[var.scenario][each.value.name].machine_type
    image_type      = "cos_containerd"
    service_account = data.google_compute_default_service_account.default.email
    tags            = ["gke-node", "${var.project_id}-gke"]
    labels = {
      env = var.project_id
    }
    metadata = {
      disable-legacy-endpoints = "true"
    }
    kubelet_config {
      cpu_manager_policy = ""
      cpu_cfs_quota      = false
      pod_pids_limit     = 0
    }
  }
  lifecycle {
    ignore_changes = [
      management[0].auto_repair,
      management[0].auto_upgrade,
      timeouts,
      node_config[0].resource_labels
    ]
  }
}
