locals {
  deployments = {
    primary = {
      subdomain = "${var.name}-scaletest"
      url = "http://${var.name}-scaletest.${var.cloudflare_domain}"
      region = "us-east1"
      zone   = "us-east1-c"
      cidr   = "10.200.0.0/24"
    }
    europe = {
      subdomain = "${var.name}-europe-scaletest"
      url = "http://${var.name}-europe-scaletest.${var.cloudflare_domain}"
      region = "europe-west1"
      zone   = "europe-west1-b"
      cidr   = "10.201.0.0/24"
    }
    asia = {
      subdomain = "${var.name}-asia-scaletest"
      url = "http://${var.name}-asia-scaletest.${var.cloudflare_domain}"
      region = "asia-southeast1"
      zone   = "asia-southeast1-a"
      cidr   = "10.202.0.0/24"
    }
  }
}
