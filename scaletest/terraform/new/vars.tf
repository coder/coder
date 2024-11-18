variable "project_id" {
  description = "The project in which to provision resources"
}

variable "name" {
  description = "The name all resources will be prefixed with"
}

variable "deployments" {
  type = list(object({
    name = string
    region = string
    zone   = string
    subnet_cidr = string
    coder_node_pool_size = number
    workspaces_node_pool_size = number
    misc_node_pool_size = number
  }))
}

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

// Preemptible nodes are way cheaper, but can be pulled out
// from under you at any time. Caveat emptor.
variable "node_preemptible" {
  description = "Use preemptible nodes."
  default     = false
}

// These variables control the node pool dedicated to Coder.
variable "nodepool_machine_type_coder" {
  description = "Machine type to use for Coder control plane nodepool."
  default     = "t2d-standard-4"
}

// These variables control the size of the database to be used by Coder.
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
