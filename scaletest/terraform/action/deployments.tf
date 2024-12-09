locals {
  deployments = {
    primary = {
      subdomain = "${var.name}-scaletest"
      url       = "http://${var.name}-scaletest.${var.cloudflare_domain}"
      region    = "us-east1"
      zone      = "us-east1-c"
      cidr      = "10.200.0.0/24"
    }
    europe = {
      subdomain = "${var.name}-europe-scaletest"
      url       = "http://${var.name}-europe-scaletest.${var.cloudflare_domain}"
      region    = "europe-west1"
      zone      = "europe-west1-b"
      cidr      = "10.201.0.0/24"
    }
    asia = {
      subdomain = "${var.name}-asia-scaletest"
      url       = "http://${var.name}-asia-scaletest.${var.cloudflare_domain}"
      region    = "asia-southeast1"
      zone      = "asia-southeast1-a"
      cidr      = "10.202.0.0/24"
    }
  }

  scenarios = {
    small = {
      coder = {
        nodepool_size = 1
        machine_type  = "t2d-standard-4"
        replicas      = 1
        cpu_request   = "1000m"
        mem_request   = "6Gi"
        cpu_limit     = "2000m"
        mem_limit     = "12Gi"
      }
      provisionerd = {
        replicas    = 1
        cpu_request = "100m"
        mem_request = "1Gi"
        cpu_limit   = "1000m"
        mem_limit   = "1Gi"
      }
      workspaces = {
        nodepool_size = 1
        machine_type  = "t2d-standard-4"
        cpu_request   = "100m"
        mem_request   = "128Mi"
        cpu_limit     = "100m"
        mem_limit     = "128Mi"
      }
      misc = {
        nodepool_size = 1
        machine_type  = "t2d-standard-4"
      }
      cloudsql = {
        tier            = "db-f1-micro"
        max_connections = 500
      }
    }
  }
}
