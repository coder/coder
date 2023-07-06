nodepool_machine_type_coder      = "t2d-standard-4"
nodepool_machine_type_workspaces = "t2d-standard-4"
coder_cpu_request                = "1000m"
coder_mem_request                = "6Gi"
coder_cpu_limit                  = "2000m" # Leaving 2 CPUs for system workloads
coder_mem_limit                  = "12Gi"  # Leaving 4GB for system workloads
