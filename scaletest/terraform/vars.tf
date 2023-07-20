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

variable "k8s_version" {
  description = "Kubernetes vversion to provision."
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

// These variables control the Coder deployment.
variable "coder_replicas" {
  description = "Number of Coder replicas to provision."
  default     = 1
}

// Ensure that requests allow for at least two replicas to be scheduled
// on a single node temporarily, otherwise deployments may fail due to
// lack of resources.
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

// Allow independently scaling provisionerd resources
variable "provisionerd_cpu_request" {
  description = "CPU request to allocate to provisionerd."
  default     = "500m"
}

variable "provisionerd_mem_request" {
  description = "Memory request to allocate to provisionerd."
  default     = "512Mi"
}

variable "provisionerd_cpu_limit" {
  description = "CPU limit to allocate to provisionerd."
  default     = "1000m"
}

variable "provisionerd_mem_limit" {
  description = "Memory limit to allocate to provisionerd."
  default     = "1024Mi"
}

variable "provisionerd_replicas" {
  description = "Number of Provisionerd replicas."
  default     = 1
}

variable "provisionerd_concurrency" {
  description = "Number of concurrent provisioner jobs per provisionerd instance."
  default     = 3
}

variable "coder_chart_version" {
  description = "Version of the Coder Helm chart to install. Defaults to latest."
  default     = null
}

variable "coder_image_repo" {
  description = "Repository to use for Coder image."
  default     = "ghcr.io/coder/coder"
}

variable "coder_image_tag" {
  description = "Tag to use for Coder image."
  default     = "latest"
}

variable "coder_experiments" {
  description = "Coder Experiments to enable."
  default     = ""
}

// These variables control the default workspace template.
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

// These variables control the Prometheus deployment.
variable "prometheus_remote_write_user" {
  description = "Username for Prometheus remote write."
  default     = ""
}

variable "prometheus_remote_write_password" {
  description = "Password for Prometheus remote write."
  default     = ""
}

variable "prometheus_remote_write_url" {
  description = "URL for Prometheus remote write. Defaults to stats.dev.c8s.io."
  default     = "https://stats.dev.c8s.io:9443/api/v1/write"
}

variable "prometheus_remote_write_insecure_skip_verify" {
  description = "Skip TLS verification for Prometheus remote write."
  default     = true
}

variable "prometheus_remote_write_metrics_regex" {
  description = "Allowlist regex of metrics for Prometheus remote write."
  default     = ".*"
}

variable "prometheus_remote_write_send_interval" {
  description = "Prometheus remote write interval."
  default     = "15s"
}
