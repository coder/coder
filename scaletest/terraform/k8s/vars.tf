variable "state" {
  description = "The state of the cluster. Valid values are 'started', and 'stopped'."
  validation {
    condition     = contains(["started", "stopped"], var.state)
    error_message = "value must be one of 'started' or 'stopped'"
  }
  default = "started"
}

variable "name" {
  description = "Adds a prefix to resources."
}

variable "kubernetes_kubeconfig_path" {
  description = "Path to kubeconfig to use to provision resources."
}

variable "kubernetes_nodepool_coder" {
  description = "Name of the nodepool on which to run Coder."
}

variable "kubernetes_nodepool_workspaces" {
  description = "Name of the nodepool on which to run workspaces."
}

variable "kubernetes_nodepool_misc" {
  description = "Name of the nodepool on which to run everything else."
}

// These variables control the Coder deployment.
variable "coder_access_url" {
  description = "Access URL for the Coder deployment."
}
variable "coder_replicas" {
  description = "Number of Coder replicas to provision."
  default     = 1
}

variable "coder_address" {
  description = "IP address to use for Coder service."
}

variable "coder_db_url" {
  description = "URL of the database for Coder to use."
  sensitive   = true
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
variable "prometheus_external_label_cluster" {
  description = "Value for the Prometheus external label named cluster."
}

variable "prometheus_postgres_dbname" {
  description = "Database for Postgres to monitor."
}

variable "prometheus_postgres_host" {
  description = "Database hostname for Prometheus."
}

variable "prometheus_postgres_password" {
  description = "Postgres password for Prometheus."
  sensitive   = true
}

variable "prometheus_postgres_user" {
  description = "Postgres username for Prometheus."
}

variable "prometheus_remote_write_user" {
  description = "Username for Prometheus remote write."
  default     = ""
}

variable "prometheus_remote_write_password" {
  description = "Password for Prometheus remote write."
  default     = ""
  sensitive   = true
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
