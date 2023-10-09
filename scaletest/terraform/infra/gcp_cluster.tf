data "google_compute_default_service_account" "default" {
  project = var.project_id
}

locals {
  abs_module_path         = abspath(path.module)
  rel_kubeconfig_path     = "../../.coderv2/${var.name}-cluster.kubeconfig"
  cluster_kubeconfig_path = abspath("${local.abs_module_path}/${local.rel_kubeconfig_path}")
}

resource "google_container_cluster" "primary" {
  name                      = var.name
  location                  = var.zone
  project                   = var.project_id
  network                   = google_compute_network.vpc.name
  subnetwork                = google_compute_subnetwork.subnet.name
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

resource "google_container_node_pool" "coder" {
  name     = "${var.name}-coder"
  location = var.zone
  project  = var.project_id
  cluster  = google_container_cluster.primary.name
  autoscaling {
    min_node_count = 1
    max_node_count = var.nodepool_size_coder
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

resource "google_container_node_pool" "workspaces" {
  name     = "${var.name}-workspaces"
  location = var.zone
  project  = var.project_id
  cluster  = google_container_cluster.primary.name
  autoscaling {
    min_node_count       = 0
    total_max_node_count = var.nodepool_size_workspaces
  }
  management {
    auto_upgrade = false
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
    machine_type    = var.nodepool_machine_type_workspaces
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

resource "google_container_node_pool" "misc" {
  name       = "${var.name}-misc"
  location   = var.zone
  project    = var.project_id
  cluster    = google_container_cluster.primary.name
  node_count = var.state == "stopped" ? 0 : var.nodepool_size_misc
  management {
    auto_upgrade = false
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
    machine_type    = var.nodepool_machine_type_misc
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

resource "null_resource" "cluster_kubeconfig" {
  depends_on = [google_container_cluster.primary]
  triggers = {
    path       = local.cluster_kubeconfig_path
    name       = google_container_cluster.primary.name
    project_id = var.project_id
    zone       = var.zone
  }
  provisioner "local-exec" {
    command = <<EOF
      KUBECONFIG=${self.triggers.path} gcloud container clusters get-credentials ${self.triggers.name} --project=${self.triggers.project_id} --zone=${self.triggers.zone}
    EOF
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<EOF
      rm -f ${self.triggers.path}
    EOF
  }
}
