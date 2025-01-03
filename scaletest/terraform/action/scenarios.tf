locals {
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
    medium = {
      coder = {
        nodepool_size = 1
        machine_type  = "t2d-standard-8"
        replicas      = 1
        cpu_request   = "3000m"
        mem_request   = "12Gi"
        cpu_limit     = "6000m"
        mem_limit     = "24Gi"
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
        machine_type  = "t2d-standard-8"
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
        tier            = "db-custom-1-3840"
        max_connections = 500
      }
    }
    large = {
      coder = {
        nodepool_size = 3
        machine_type  = "t2d-standard-8"
        replicas      = 3
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
        machine_type  = "t2d-standard-8"
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
        tier            = "db-custom-2-7680"
        max_connections = 500
      }
    }
  }
}
