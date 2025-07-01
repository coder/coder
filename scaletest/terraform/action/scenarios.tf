locals {
  scenarios = {
    large = {
      coder = {
        nodepool_size = 3
        machine_type  = "c2d-standard-8"
        replicas      = 3
        cpu_request   = "4000m"
        mem_request   = "12Gi"
        cpu_limit     = "4000m"
        mem_limit     = "12Gi"
      }
      provisionerd = {
        replicas    = 30
        cpu_request = "100m"
        mem_request = "256Mi"
        cpu_limit   = "1000m"
        mem_limit   = "1Gi"
      }
      workspaces = {
        count_per_deployment = 100
        nodepool_size        = 3
        machine_type         = "c2d-standard-32"
        cpu_request          = "100m"
        mem_request          = "128Mi"
        cpu_limit            = "100m"
        mem_limit            = "128Mi"
      }
      misc = {
        nodepool_size = 1
        machine_type  = "c2d-standard-32"
      }
      cloudsql = {
        tier            = "db-custom-2-7680"
        max_connections = 500
      }
    }
  }
}
