locals {
  project_apis = [
    "cloudtrace",
    "compute",
    "container",
    "logging",
    "monitoring",
    "servicemanagement",
    "servicenetworking",
    "sqladmin",
    "stackdriver",
    "storage-api",
  ]
}

data "google_project" "project" {
  project_id = var.project_id
}

resource "google_project_service" "api" {
  for_each = toset(local.project_apis)
  project  = data.google_project.project.project_id
  service  = "${each.value}.googleapis.com"

  disable_dependent_services = false
  disable_on_destroy         = false
}
