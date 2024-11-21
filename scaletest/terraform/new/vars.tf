variable "name" {
  description = "The name all resources will be prefixed with"
}

// GCP
variable "project_id" {
  description = "The project in which to provision resources"
}

# variable "deployments" {
#   type = list(object({
#     name = string
#     region = string
#     zone   = string
#     subnet_cidr = string
#     coder_node_pool_size = number
#     workspaces_node_pool_size = number
#     misc_node_pool_size = number
#   }))
# }

variable "k8s_version" {
  description = "Kubernetes version to provision."
  default     = "1.24"
}

variable "node_disk_size_gb" {
  description = "Size of the root disk for cluster nodes."
  default     = 100
}

variable "node_image_type" {
  description = "Image type to use for cluster nodes."
  default     = "cos_containerd"
}

variable "node_preemptible" {
  description = "Use preemptible nodes."
  default     = false
}

variable "nodepool_machine_type_coder" {
  description = "Machine type to use for Coder control plane nodepool."
  default     = "t2d-standard-4"
}

variable "cloudsql_version" {
  description = "CloudSQL version to provision"
  default     = "POSTGRES_14"
}

variable "cloudsql_tier" {
  description = "CloudSQL database tier."
  default     = "db-f1-micro"
}

variable "cloudsql_max_connections" {
  description = "CloudSQL database max_connections"
  default     = 500
}

// Cloudflare
variable "cloudflare_api_token" {
  description = "Cloudflare API token."
  sensitive   = true
}

variable "cloudflare_email" {
  description = "Cloudflare email address."
  sensitive   = true
}

variable "cloudflare_domain" {
  description = "Cloudflare coder domain."
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID."
}

// Coder
variable "coder_chart_version" {
  description = "Version of the Coder Helm chart to install. Defaults to latest."
  default     = null
}

variable "coder_image_tag" {
  description = "Tag to use for Coder image."
  default     = "latest"
}

variable "coder_image_repo" {
  description = "Repository to use for Coder image."
  default     = "ghcr.io/coder/coder"
}

variable "coder_replicas" {
  description = "Number of Coder replicas to provision."
  default     = 1
}

variable "coder_license" {
  description = "Coder license key."
  # sensitive   = true
  
}

variable "coder_cpu_request" {
  description = "CPU request to allocate to Coder."
  default     = "500m"
}

variable "coder_mem_request" {
  description = "Memory request to allocate to Coder."
  default     = "512Mi"
}

variable "coder_cpu_limit" {
  description = "CPU limit to allocate to Coder."
  default     = "1000m"
}

variable "coder_mem_limit" {
  description = "Memory limit to allocate to Coder."
  default     = "1024Mi"
}

variable "coder_experiments" {
  description = "Coder Experiments to enable."
  default     = ""
}

// Workspaces
variable "workspace_image" {
  description = "Image and tag to use for workspaces."
  default     = "docker.io/codercom/enterprise-minimal:ubuntu"
}

variable "workspace_cpu_request" {
  description = "CPU request to allocate to workspaces."
  default     = "100m"
}

variable "workspace_cpu_limit" {
  description = "CPU limit to allocate to workspaces."
  default     = "100m"
}

variable "workspace_mem_request" {
  description = "Memory request to allocate to workspaces."
  default     = "128Mi"
}

variable "workspace_mem_limit" {
  description = "Memory limit to allocate to workspaces."
  default     = "128Mi"
}

// Provisioners
variable "provisionerd_cpu_request" {
  description = "CPU request to allocate to provisionerd."
  default     = "100m"
}

variable "provisionerd_mem_request" {
  description = "Memory request to allocate to provisionerd."
  default     = "1Gi"
}

variable "provisionerd_cpu_limit" {
  description = "CPU limit to allocate to provisionerd."
  default     = "1000m"
}

variable "provisionerd_mem_limit" {
  description = "Memory limit to allocate to provisionerd."
  default     = "1Gi"
}

variable "provisionerd_replicas" {
  description = "Number of Provisionerd replicas."
  default     = 1
}

variable "provisionerd_chart_version" {
  description = "Version of the Provisionerd Helm chart to install. Defaults to latest."
  default     = null
}

variable "provisionerd_image_repo" {
  description = "Repository to use for Provisionerd image."
  default     = "ghcr.io/coder/coder"
}

variable "provisionerd_image_tag" {
  description = "Tag to use for Provisionerd image."
  default     = "latest"
}
