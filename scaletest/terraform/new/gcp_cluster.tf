data "google_compute_default_service_account" "default" {
  project = var.project_id
  depends_on = [ google_project_service.api["compute.googleapis.com"] ]
}

locals {
  node_pools = flatten([ for i, deployment in var.deployments : [
    {
      name = "${var.name}-${deployment.name}-coder"
      zone = deployment.zone
      size = deployment.coder_node_pool_size
      cluster_i = i
    },     
    {
      name = "${var.name}-${deployment.name}-workspaces"
      zone = deployment.zone
      size = deployment.workspaces_node_pool_size
      cluster_i = i
    },
    {
      name = "${var.name}-${deployment.name}-misc"
      zone = deployment.zone
      size = deployment.misc_node_pool_size
      cluster_i = i
    }
  ] ])
}

resource "google_container_cluster" "cluster" {
  count                     = length(var.deployments)
  name                      = "${var.name}-${var.deployments[count.index].name}"
  location                  = var.deployments[count.index].zone
  project                   = var.project_id
  network                   = google_compute_network.vpc.name
  subnetwork                = google_compute_subnetwork.subnet[count.index].name
  networking_mode           = "VPC_NATIVE"
  default_max_pods_per_node = 256
  ip_allocation_policy { # Required with networking_mode=VPC_NATIVE

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
  count    = length(local.node_pools)
  name     = local.node_pools[count.index].name
  location = local.node_pools[count.index].zone
  project  = var.project_id
  cluster  = google_container_cluster.cluster[local.node_pools[count.index].cluster_i].name
  autoscaling {
    min_node_count = 1
    max_node_count = local.node_pools[count.index].size
  }
  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/trace.append",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/servicecontrol",
    ]
    disk_size_gb    = var.node_disk_size_gb
    machine_type    = var.nodepool_machine_type_coder
    image_type      = var.node_image_type
    preemptible     = var.node_preemptible
    service_account = data.google_compute_default_service_account.default.email
    tags            = ["gke-node", "${var.project_id}-gke"]
    labels = {
      env = var.project_id
    }
    metadata = {
      disable-legacy-endpoints = "true"
    }
  }
  lifecycle {
    ignore_changes = [management[0].auto_repair, management[0].auto_upgrade, timeouts]
  }
}
