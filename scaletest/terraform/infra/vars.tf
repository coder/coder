variable "state" {
  description = "The state of the cluster. Valid values are 'started', and 'stopped'."
  validation {
    condition     = contains(["started", "stopped"], var.state)
    error_message = "value must be one of 'started' or 'stopped'"
  }
  default = "started"
}

variable "project_id" {
  description = "The project in which to provision resources"
}

variable "name" {
  description = "Adds a prefix to resources."
}

variable "region" {
  description = "GCP region in which to provision resources."
  default     = "us-east1"
}

variable "zone" {
  description = "GCP zone in which to provision resources."
  default     = "us-east1-c"
}

variable "subnet_cidr" {
  description = "CIDR range for the subnet."
  default     = "10.200.0.0/24"
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

// We create three nodepools:
// - One for the Coder control plane
// - One for workspaces
// - One for everything else (for example, load generation)

// These variables control the node pool dedicated to Coder.
variable "nodepool_machine_type_coder" {
  description = "Machine type to use for Coder control plane nodepool."
  default     = "t2d-standard-4"
}

variable "nodepool_size_coder" {
  description = "Number of cluster nodes for the Coder control plane nodepool."
  default     = 1
}

// These variables control the node pool dedicated to workspaces.
variable "nodepool_machine_type_workspaces" {
  description = "Machine type to use for the workspaces nodepool."
  default     = "t2d-standard-4"
}

variable "nodepool_size_workspaces" {
  description = "Number of cluster nodes for the workspaces nodepool."
  default     = 1
}

// These variables control the node pool for everything else.
variable "nodepool_machine_type_misc" {
  description = "Machine type to use for the misc nodepool."
  default     = "t2d-standard-4"
}

variable "nodepool_size_misc" {
  description = "Number of cluster nodes for the misc nodepool."
  default     = 1
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
