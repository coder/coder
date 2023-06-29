nodepool_machine_type_coder      = "t2d-standard-8"
nodepool_machine_type_workspaces = "t2d-standard-8"
cloudsql_tier                    = "db-custom-1-3840"
coder_cpu_request                = "3000m"
coder_mem_request                = "12Gi"
coder_cpu_limit                  = "6000m" # Leaving 2 CPUs for system workloads
coder_mem_limit                  = "24Gi"  # Leaving 8 GB for system workloads
