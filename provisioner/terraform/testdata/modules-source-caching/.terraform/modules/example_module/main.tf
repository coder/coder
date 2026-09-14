terraform {
  required_version = ">= 1.0"

  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 0.12"
    }
  }
}

variable "url" {
  description = "The URL of the Git repository."
  type        = string
}

variable "base_dir" {
  default     = ""
  description = "The base directory to clone the repository. Defaults to \"$HOME\"."
  type        = string
}

variable "agent_id" {
  description = "The ID of a Coder agent."
  type        = string
}

variable "git_providers" {
  type = map(object({
    provider = string
  }))
  description = "A mapping of URLs to their git provider."
  default = {
    "https://github.com/" = {
      provider = "github"
    },
    "https://gitlab.com/" = {
      provider = "gitlab"
    },
  }
  validation {
    error_message = "Allowed values for provider are \"github\" or \"gitlab\"."
    condition     = alltrue([for provider in var.git_providers : contains(["github", "gitlab"], provider.provider)])
  }
}

variable "branch_name" {
  description = "The branch name to clone. If not provided, the default branch will be cloned."
  type        = string
  default     = ""
}

variable "folder_name" {
  description = "The destination folder to clone the repository into."
  type        = string
  default     = ""
}

locals {
  # Remove query parameters and fragments from the URL
  url = replace(replace(var.url, "/\\?.*/", ""), "/#.*/", "")

  # Find the git provider based on the URL and determine the tree path
  provider_key = try(one([for key in keys(var.git_providers) : key if startswith(local.url, key)]), null)
  provider     = try(lookup(var.git_providers, local.provider_key).provider, "")
  tree_path    = local.provider == "gitlab" ? "/-/tree/" : local.provider == "github" ? "/tree/" : ""

  # Remove tree and branch name from the URL
  clone_url = var.branch_name == "" && local.tree_path != "" ? replace(local.url, "/${local.tree_path}.*/", "") : local.url
  # Extract the branch name from the URL
  branch_name = var.branch_name == "" && local.tree_path != "" ? replace(replace(local.url, local.clone_url, ""), "/.*${local.tree_path}/", "") : var.branch_name
  # Extract the folder name from the URL
  folder_name = var.folder_name == "" ? replace(basename(local.clone_url), ".git", "") : var.folder_name
  # Construct the path to clone the repository
  clone_path = var.base_dir != "" ? join("/", [var.base_dir, local.folder_name]) : join("/", ["~", local.folder_name])
  # Construct the web URL
  web_url = startswith(local.clone_url, "git@") ? replace(replace(local.clone_url, ":", "/"), "git@", "https://") : local.clone_url
}

output "repo_dir" {
  value       = local.clone_path
  description = "Full path of cloned repo directory"
}

output "git_provider" {
  value       = local.provider
  description = "The git provider of the repository"
}

output "folder_name" {
  value       = local.folder_name
  description = "The name of the folder that will be created"
}

output "clone_url" {
  value       = local.clone_url
  description = "The exact Git repository URL that will be cloned"
}

output "web_url" {
  value       = local.web_url
  description = "Git https repository URL (may be invalid for unsupported providers)"
}

output "branch_name" {
  value       = local.branch_name
  description = "Git branch name (may be empty)"
}

resource "coder_script" "git_clone" {
  agent_id = var.agent_id
  script = templatefile("${path.module}/run.sh", {
    CLONE_PATH = local.clone_path,
    REPO_URL : local.clone_url,
    BRANCH_NAME : local.branch_name,
  })
  display_name       = "Git Clone"
  icon               = "/icon/git.svg"
  run_on_start       = true
  start_blocks_login = true
}
