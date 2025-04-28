resource "google_sql_database_instance" "db" {
  name                = "${var.name}-coder"
  project             = var.project_id
  region              = local.deployments.primary.region
  database_version    = "POSTGRES_14"
  deletion_protection = false

  depends_on = [google_service_networking_connection.private_vpc_connection]

  settings {
    tier              = local.scenarios[var.scenario].cloudsql.tier
    activation_policy = "ALWAYS"
    availability_type = "ZONAL"

    location_preference {
      zone = local.deployments.primary.zone
    }

    database_flags {
      name  = "max_connections"
      value = local.scenarios[var.scenario].cloudsql.max_connections
    }

    ip_configuration {
      ipv4_enabled    = false
      private_network = local.vpc_id
    }

    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = false
      record_client_address   = false
    }
  }

  lifecycle {
    ignore_changes = [deletion_protection, timeouts]
  }
}

resource "google_sql_database" "coder" {
  project  = var.project_id
  instance = google_sql_database_instance.db.id
  name     = "${var.name}-coder"
  # required for postgres, otherwise db fails to delete
  deletion_policy = "ABANDON"
  lifecycle {
    ignore_changes = [deletion_policy]
  }
}

resource "random_password" "coder_postgres_password" {
  length = 12
}

resource "random_password" "prometheus_postgres_password" {
  length = 12
}

resource "google_sql_user" "coder" {
  project  = var.project_id
  instance = google_sql_database_instance.db.id
  name     = "${var.name}-coder"
  type     = "BUILT_IN"
  password = random_password.coder_postgres_password.result
  # required for postgres, otherwise user fails to delete
  deletion_policy = "ABANDON"
  lifecycle {
    ignore_changes = [deletion_policy, password]
  }
}

resource "google_sql_user" "prometheus" {
  project  = var.project_id
  instance = google_sql_database_instance.db.id
  name     = "${var.name}-prometheus"
  type     = "BUILT_IN"
  password = random_password.prometheus_postgres_password.result
  # required for postgres, otherwise user fails to delete
  deletion_policy = "ABANDON"
  lifecycle {
    ignore_changes = [deletion_policy, password]
  }
}

locals {
  coder_db_url = "postgres://${google_sql_user.coder.name}:${urlencode(random_password.coder_postgres_password.result)}@${google_sql_database_instance.db.private_ip_address}/${google_sql_database.coder.name}?sslmode=disable"
}
