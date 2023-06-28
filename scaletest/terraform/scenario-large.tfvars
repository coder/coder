nodepool_machine_type_coder      = "t2d-standard-8"
nodepool_size_coder              = 3
nodepool_machine_type_workspaces = "t2d-standard-8"
cloudsql_tier                    = "db-custom-4-7680"
coder_cpu                        = "6000m" # Leaving 2 CPUs for system workloads
coder_mem                        = "24Gi"  # Leaving 8 GB for system workloads
coder_replicas                   = 3
