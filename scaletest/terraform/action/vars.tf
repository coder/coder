variable "name" {
  description = "The name all resources will be prefixed with"
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
}

variable "k8s_version" {
  description = "Kubernetes version to provision."
  default     = "1.24"
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

// Prometheus
variable "prometheus_remote_write_url" {
  description = "URL to push prometheus metrics to."
}
