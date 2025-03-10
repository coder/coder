locals {
  vpc_name    = "scaletest"
  vpc_id      = "projects/${var.project_id}/global/networks/${local.vpc_name}"
  subnet_name = "scaletest"
}

resource "google_compute_address" "coder" {
  for_each     = local.deployments
  project      = var.project_id
  region       = each.value.region
  name         = "${var.name}-${each.key}-coder"
  address_type = "EXTERNAL"
  network_tier = "PREMIUM"
}

resource "google_compute_global_address" "sql_peering" {
  project       = var.project_id
  name          = "${var.name}-sql-peering"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = local.vpc_name
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = local.vpc_id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.sql_peering.name]
}
