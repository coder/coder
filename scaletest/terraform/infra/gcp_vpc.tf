resource "google_compute_network" "vpc" {
  project                 = var.project_id
  name                    = var.name
  auto_create_subnetworks = "false"
  depends_on = [
    google_project_service.api["compute.googleapis.com"]
  ]
}

resource "google_compute_subnetwork" "subnet" {
  name          = var.name
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.name
  ip_cidr_range = var.subnet_cidr
}

resource "google_compute_global_address" "sql_peering" {
  project       = var.project_id
  name          = "${var.name}-sql-peering"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
}

resource "google_compute_address" "coder" {
  project      = var.project_id
  region       = var.region
  name         = "${var.name}-coder"
  address_type = "EXTERNAL"
  network_tier = "PREMIUM"
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.sql_peering.name]
}
