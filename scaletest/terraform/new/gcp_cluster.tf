data "google_compute_default_service_account" "default" {
  project = var.project_id
  depends_on = [ google_project_service.api["compute.googleapis.com"] ]
}

locals {
  clusters = {
    primary = {
      region = "us-east1"
      zone   = "us-east1-c"
      cidr   = "10.200.0.0/24"
    }
  }
  node_pools = {
    primary_coder = {
      name = "coder"
      cluster = "primary"
      size = 1
    }
    primary_workspaces = {
      name = "workspaces"
      cluster = "primary"
      size = 1
    }
    primary_misc = {
      name = "misc"
      cluster = "primary"
      size = 1
    }
  }
}

resource "google_container_cluster" "cluster" {
  for_each                  = local.clusters
  name                      = "${var.name}-${each.key}"
  location                  = each.value.zone
  project                   = var.project_id
  network                   = google_compute_network.vpc.name
  subnetwork                = google_compute_subnetwork.subnet[each.key].name
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
  for_each = local.node_pools
  name     = each.value.name
  location = local.clusters[each.value.cluster].zone
  project  = var.project_id
  cluster  = google_container_cluster.cluster[each.value.cluster].name
  autoscaling {
    min_node_count = 1
    max_node_count = each.value.size
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
