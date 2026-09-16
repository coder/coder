variable "azure_subscription_id" {
  description = "Azure subscription used by the external Terraform provisioner. Supply its ARM_* authentication separately."
  type        = string
  validation {
    condition     = can(regex("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$", var.azure_subscription_id))
    error_message = "Supply an Azure subscription UUID."
  }
}

variable "azure_location" {
  description = "Azure region with quota for the selected VM size and Premium managed disks."
  type        = string
  default     = "eastus"
}

variable "azure_vm_size" {
  description = "Host VM size. Use at least 4 vCPU and 16 GiB RAM; B-series CPU credits limit performance experiments."
  type        = string
  default     = "Standard_B4ms"
  validation {
    condition     = can(regex("^Standard_[A-Za-z0-9_]+$", var.azure_vm_size))
    error_message = "Supply a Standard Azure VM size available in the chosen region."
  }
}

variable "provisioner_tags" {
  description = "Nonsecret workspace routing tags for external Terraform provisioners with Azure access. Also pass matching --provisioner-tag values when importing this template."
  type        = map(string)
  default     = {}
}

variable "coder_source_repository" {
  description = "Public GitHub repository containing the experimental native sandbox implementation. No authentication is forwarded to the host."
  type        = string
  default     = "https://github.com/coder/coder.git"
  validation {
    condition     = can(regex("^https://github\\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\\.git$", var.coder_source_repository))
    error_message = "Supply a public https://github.com/owner/repository.git URL without credentials, query parameters, or fragments."
  }
}

variable "coder_source_commit" {
  description = "Full public commit SHA containing this experiment, cmd/coder, and provisioner/sandbox/testhost. Required to avoid a mutable or recursively embedded source patch."
  type        = string
  validation {
    condition     = can(regex("^[a-f0-9]{40}$", var.coder_source_commit))
    error_message = "Supply the full lowercase 40-character commit SHA of the reviewed experimental source."
  }
}
