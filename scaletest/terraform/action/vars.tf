variable "name" {
  description = "The name all resources will be prefixed with. Must be one of alpha, bravo, or charlie."
  validation {
    condition     = contains(["alpha", "bravo", "charlie"], var.name)
    error_message = "Name must be one of alpha, bravo, or charlie."
  }
}

variable "scenario" {
  description = "The scenario to deploy"
  validation {
    condition     = contains(["small", "medium", "large"], var.scenario)
    error_message = "Scenario must be one of small, medium, or large"
  }
}

// GCP
variable "project_id" {
  description = "The project in which to provision resources"
  default     = "coder-scaletest"
}

variable "k8s_version" {
  description = "Kubernetes version to provision."
  default     = "1.24"
}

// Cloudflare
variable "cloudflare_api_token" {
  description = "Cloudflare API token."
  sensitive   = true
  # only override if you want to change the cloudflare_domain; pulls the token for scaletest.dev from Google Secrets
  # Manager if null.
  default = null
}

variable "cloudflare_domain" {
  description = "Cloudflare coder domain."
  default     = "scaletest.dev"
}

// Coder
variable "coder_license" {
  description = "Coder license key."
  sensitive   = true
}

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

variable "coder_experiments" {
  description = "Coder Experiments to enable."
  default     = ""
}

// Workspaces
variable "workspace_image" {
  description = "Image and tag to use for workspaces."
  default     = "docker.io/codercom/enterprise-minimal:ubuntu"
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

variable "observability_cluster_name" {
  description = "Name of the observability GKE cluster."
  default     = "observability"
}

variable "observability_cluster_location" {
  description = "Location of the observability GKE cluster."
  default     = "us-east1-b"
}

variable "observability_cluster_vpc" {
  description = "Name of the observability cluster VPC network to peer with."
  default     = "default"
}

variable "cloudflare_api_token_secret" {
  description = "Name of the Google Secret Manager secret containing the Cloudflare API token."
  default     = "cloudflare-api-token-dns"
}

// Prometheus
variable "prometheus_remote_write_url" {
  description = "URL to push prometheus metrics to."
}
