module "docker" {
  source                   = "cdr.jfrog.io/tf__main/docker/docker"
  jfrog_host               = var.jfrog_host
  artifactory_access_token = var.artifactory_access_token
}

variable "jfrog_host" {
  type        = string
  description = "JFrog instance hostname. For example, 'YYY.jfrog.io'."
}

variable "artifactory_access_token" {
  type        = string
  description = "The admin-level access token to use for JFrog."
}

