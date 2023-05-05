data "google_compute_network" "default" {
  project = var.project_id
  name    = "default"
}

data "google_compute_global_address" "sql_peering" {
  name = "sql-ip-address"
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = data.google_compute_network.default.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.sql_peering.name]
}

resource "google_sql_database_instance" "db" {
  name             = "${var.name}-db"
  region           = var.region
  database_version = var.cloudsql_version

  depends_on = [google_service_networking_connection.private_vpc_connection]

  settings {
    tier              = var.cloudsql_tier
    activation_policy = "ALWAYS"
    availability_type = "ZONAL"

    location_preference {
      zone = var.zone
    }

    database_flags {
      name  = "max_connections"
      value = "500"
    }

    ip_configuration {
      ipv4_enabled    = false
      private_network = data.google_compute_network.default.id
    }

    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = false
      record_client_address   = false
    }
  }
}
