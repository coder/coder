locals {
  # Generate a /14 for each deployment.
  cidr_networks = cidrsubnets(
    "172.16.0.0/12",
    2,
    2,
    2,
  )

  networks = {
    alpha   = local.cidr_networks[0]
    bravo   = local.cidr_networks[1]
    charlie = local.cidr_networks[2]
  }

  # Generate a bunch of /18s within the subnet we're using from the above map.
  cidr_subnetworks = cidrsubnets(
    local.networks[var.name],
    4, # PSA
    4, # primary subnetwork
    4, # primary k8s pod network
    4, # primary k8s services network
    4, # europe subnetwork
    4, # europe k8s pod network
    4, # europe k8s services network
    4, # asia subnetwork
    4, # asia k8s pod network
    4, # asia k8s services network
  )

  psa_range_address       = split("/", local.cidr_subnetworks[0])[0]
  psa_range_prefix_length = tonumber(split("/", local.cidr_subnetworks[0])[1])

  subnetworks = {
    primary = local.cidr_subnetworks[1]
    europe  = local.cidr_subnetworks[4]
    asia    = local.cidr_subnetworks[7]
  }
  cluster_ranges = {
    primary = {
      pods     = local.cidr_subnetworks[2]
      services = local.cidr_subnetworks[3]
    }
    europe = {
      pods     = local.cidr_subnetworks[5]
      services = local.cidr_subnetworks[6]
    }
    asia = {
      pods     = local.cidr_subnetworks[8]
      services = local.cidr_subnetworks[9]
    }
  }

  secondary_ip_range_k8s_pods     = "k8s-pods"
  secondary_ip_range_k8s_services = "k8s-services"
}

# Create a VPC for the deployment
resource "google_compute_network" "network" {
  project                 = var.project_id
  name                    = "${var.name}-scaletest"
  description             = "scaletest network for ${var.name}"
  auto_create_subnetworks = false
}

# Create a subnetwork with a unique range for each region
resource "google_compute_subnetwork" "subnetwork" {
  for_each = local.subnetworks
  name     = "${var.name}-${each.key}"
  # Use the deployment region
  region                   = local.deployments[each.key].region
  network                  = google_compute_network.network.id
  project                  = var.project_id
  ip_cidr_range            = each.value
  private_ip_google_access = true

  secondary_ip_range {
    range_name    = local.secondary_ip_range_k8s_pods
    ip_cidr_range = local.cluster_ranges[each.key].pods
  }

  secondary_ip_range {
    range_name    = local.secondary_ip_range_k8s_services
    ip_cidr_range = local.cluster_ranges[each.key].services
  }
}

# Create a public IP for each region
resource "google_compute_address" "coder" {
  for_each     = local.deployments
  project      = var.project_id
  region       = each.value.region
  name         = "${var.name}-${each.key}-coder"
  address_type = "EXTERNAL"
  network_tier = "PREMIUM"
}

# Reserve an internal range for Google-managed services (PSA), used for Cloud
# SQL
resource "google_compute_global_address" "psa_peering" {
  project       = var.project_id
  name          = "${var.name}-sql-peering"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  address       = local.psa_range_address
  prefix_length = local.psa_range_prefix_length
  network       = google_compute_network.network.self_link
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.network.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.psa_peering.name]
}

# Join the new network to the observability network so we can talk to the
# Prometheus instance
data "google_compute_network" "observability" {
  project = var.project_id
  name    = var.observability_cluster_vpc
}

resource "google_compute_network_peering" "scaletest_to_observability" {
  name                 = "peer-${google_compute_network.network.name}-to-${data.google_compute_network.observability.name}"
  network              = google_compute_network.network.self_link
  peer_network         = data.google_compute_network.observability.self_link
  import_custom_routes = true
  export_custom_routes = true
}

resource "google_compute_network_peering" "observability_to_scaletest" {
  name                 = "peer-${data.google_compute_network.observability.name}-to-${google_compute_network.network.name}"
  network              = data.google_compute_network.observability.self_link
  peer_network         = google_compute_network.network.self_link
  import_custom_routes = true
  export_custom_routes = true
}

# Allow traffic from the scaletest network into the observability network so we
# can connect to Prometheus
resource "google_compute_firewall" "observability_allow_from_scaletest" {
  project       = var.project_id
  name          = "allow-from-scaletest-${var.name}"
  network       = data.google_compute_network.observability.self_link
  direction     = "INGRESS"
  source_ranges = [local.networks[var.name]]
  allow {
    protocol = "icmp"
  }
  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
}
