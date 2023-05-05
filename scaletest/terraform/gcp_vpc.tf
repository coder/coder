resource "google_compute_network" "vpc" {
  project                 = var.project_id
  name                    = "${var.name}-vpc"
  auto_create_subnetworks = "false"
  depends_on = [
    google_project_service.api["compute.googleapis.com"]
  ]
}

resource "google_compute_subnetwork" "subnet" {
  name          = "${var.name}-subnet"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.name
  ip_cidr_range = "10.10.0.0/24"
}

resource "google_compute_global_address" "sql_peering" {
  name          = "${var.name}-sql-peering"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  # prefix_length = 16
  network       = google_compute_network.vpc.id
}
