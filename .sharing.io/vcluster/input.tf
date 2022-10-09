variable "namespace" {
  description = "namespace that will contain the workspace"
  type        = string
  default     = "coder-ws"
}

variable "k8s-version" {
  description = "Version of Kubernetes to Depoy as a Cluster"
  type        = string
  default     = "1.23.4"
}

variable "tls-san" {
  description = "Helm Chart Extra Args --tls-san=X"
  type        = string
  default     = "sanskar.pair.sharing.io"
}
