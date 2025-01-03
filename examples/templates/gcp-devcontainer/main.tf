terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    google = {
      source = "hashicorp/google"
    }
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

provider "coder" {}

provider "google" {
  zone    = module.gcp_region.value
  project = var.project_id
}

data "google_compute_default_service_account" "default" {}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

variable "project_id" {
  description = "Which Google Compute Project should your workspace live in?"
}

variable "cache_repo" {
  default     = ""
  description = "(Optional) Use a container registry as a cache to speed up builds. Example: host.tld/path/to/repo."
  type        = string
}

variable "cache_repo_docker_config_path" {
  default     = ""
  description = "(Optional) Path to a docker config.json containing credentials to the provided cache repo, if required. This will depend on your Coder setup. Example: `/home/coder/.docker/config.json`."
  sensitive   = true
  type        = string
}

module "gcp_region" {
  source  = "registry.coder.com/modules/gcp-region/coder"
  version = "1.0.12"
  regions = ["us", "europe"]
}

data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance Type"
  description  = "Select an instance type for your workspace."
  type         = "string"
  mutable      = false
  order        = 2
  default      = "e2-micro"
  option {
    name  = "e2-micro (2C, 1G)"
    value = "e2-micro"
  }
  option {
    name  = "e2-small (2C, 2G)"
    value = "e2-small"
  }
  option {
    name  = "e2-medium (2C, 2G)"
    value = "e2-medium"
  }
}

data "coder_parameter" "fallback_image" {
  default      = "codercom/enterprise-base:ubuntu"
  description  = "This image runs if the devcontainer fails to build."
  display_name = "Fallback Image"
  mutable      = true
  name         = "fallback_image"
  order        = 3
}

data "coder_parameter" "devcontainer_builder" {
  description  = <<-EOF
Image that will build the devcontainer.
Find the latest version of Envbuilder here: https://ghcr.io/coder/envbuilder
Be aware that using the `:latest` tag may expose you to breaking changes.
EOF
  display_name = "Devcontainer Builder"
  mutable      = true
  name         = "devcontainer_builder"
  default      = "ghcr.io/coder/envbuilder:latest"
  order        = 4
}

data "coder_parameter" "repo_url" {
  name         = "repo_url"
  display_name = "Repository URL"
  default      = "https://github.com/coder/envbuilder-starter-devcontainer"
  description  = "Repository URL"
  mutable      = true
}

data "local_sensitive_file" "cache_repo_dockerconfigjson" {
  count    = var.cache_repo_docker_config_path == "" ? 0 : 1
  filename = var.cache_repo_docker_config_path
}

# Be careful when modifying the below locals!
locals {
  # Ensure Coder username is a valid Linux username
  linux_user = lower(substr(data.coder_workspace_owner.me.name, 0, 32))
  # Name the container after the workspace and owner.
  container_name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # The devcontainer builder image is the image that will build the devcontainer.
  devcontainer_builder_image = data.coder_parameter.devcontainer_builder.value
  # We may need to authenticate with a registry. If so, the user will provide a path to a docker config.json.
  docker_config_json_base64 = try(data.local_sensitive_file.cache_repo_dockerconfigjson[0].content_base64, "")
  # The envbuilder provider requires a key-value map of environment variables. Build this here.
  envbuilder_env = {
    # ENVBUILDER_GIT_URL and ENVBUILDER_CACHE_REPO will be overridden by the provider
    # if the cache repo is enabled.
    "ENVBUILDER_GIT_URL" : data.coder_parameter.repo_url.value,
    # The agent token is required for the agent to connect to the Coder platform.
    "CODER_AGENT_TOKEN" : try(coder_agent.dev.0.token, ""),
    # The agent URL is required for the agent to connect to the Coder platform.
    "CODER_AGENT_URL" : data.coder_workspace.me.access_url,
    # The agent init script is required for the agent to start up. We base64 encode it here
    # to avoid quoting issues.
    "ENVBUILDER_INIT_SCRIPT" : "echo ${base64encode(try(coder_agent.dev[0].init_script, ""))} | base64 -d | sh",
    "ENVBUILDER_DOCKER_CONFIG_BASE64" : try(data.local_sensitive_file.cache_repo_dockerconfigjson[0].content_base64, ""),
    # The fallback image is the image that will run if the devcontainer fails to build.
    "ENVBUILDER_FALLBACK_IMAGE" : data.coder_parameter.fallback_image.value,
    # The following are used to push the image to the cache repo, if defined.
    "ENVBUILDER_CACHE_REPO" : var.cache_repo,
    "ENVBUILDER_PUSH_IMAGE" : var.cache_repo == "" ? "" : "true",
    # You can add other required environment variables here.
    # See: https://github.com/coder/envbuilder/?tab=readme-ov-file#environment-variables
  }
  # If we have a cached image, use the cached image's environment variables. Otherwise, just use
  # the environment variables we've defined above.
  docker_env_input = try(envbuilder_cached_image.cached.0.env_map, local.envbuilder_env)
  # Convert the above to the list of arguments for the Docker run command.
  # The startup script will write this to a file, which the Docker run command will reference.
  docker_env_list_base64 = base64encode(join("\n", [for k, v in local.docker_env_input : "${k}=${v}"]))

  # Builder image will either be the builder image parameter, or the cached image, if cache is provided.
  builder_image = try(envbuilder_cached_image.cached[0].image, data.coder_parameter.devcontainer_builder.value)

  # The GCP VM needs a startup script to set up the environment and start the container. Defining this here.
  # NOTE: make sure to test changes by uncommenting the local_file resource at the bottom of this file
  # and running `terraform apply` to see the generated script. You should also run shellcheck on the script
  # to ensure it is valid.
  startup_script = <<-META
    #!/usr/bin/env sh
    set -eux

    # If user does not exist, create it and set up passwordless sudo
    if ! id -u "${local.linux_user}" >/dev/null 2>&1; then
      useradd -m -s /bin/bash "${local.linux_user}"
      echo "${local.linux_user} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-user
    fi

    # Check for Docker, install if not present
    if ! command -v docker >/dev/null 2>&1; then
      echo "Docker not found, installing..."
      curl -fsSL https://get.docker.com -o get-docker.sh && sudo sh get-docker.sh >/dev/null 2>&1
      sudo usermod -aG docker ${local.linux_user}
      newgrp docker
    else
      echo "Docker is already installed."
    fi

    # Write the Docker config JSON to disk if it is provided.
    if [ -n "${local.docker_config_json_base64}" ]; then
      mkdir -p "/home/${local.linux_user}/.docker"
      printf "%s" "${local.docker_config_json_base64}" | base64 -d | tee "/home/${local.linux_user}/.docker/config.json"
      chown -R ${local.linux_user}:${local.linux_user} "/home/${local.linux_user}/.docker"
    fi

    # Write the container env to disk.
    printf "%s" "${local.docker_env_list_base64}" | base64 -d | tee "/home/${local.linux_user}/env.txt"

    # Start envbuilder.
    docker run \
     --rm \
     --net=host \
     -h ${lower(data.coder_workspace.me.name)} \
     -v /home/${local.linux_user}/envbuilder:/workspaces \
     -v /var/run/docker.sock:/var/run/docker.sock \
     --env-file /home/${local.linux_user}/env.txt \
    ${local.builder_image}
  META
}

# Create a persistent disk to store the workspace data.
resource "google_compute_disk" "root" {
  name  = "coder-${data.coder_workspace.me.id}-root"
  type  = "pd-ssd"
  image = "debian-cloud/debian-12"
  lifecycle {
    ignore_changes = all
  }
}

# Check for the presence of a prebuilt image in the cache repo
# that we can use instead.
resource "envbuilder_cached_image" "cached" {
  count         = var.cache_repo == "" ? 0 : data.coder_workspace.me.start_count
  builder_image = local.devcontainer_builder_image
  git_url       = data.coder_parameter.repo_url.value
  cache_repo    = var.cache_repo
  extra_env     = local.envbuilder_env
}

# This is useful for debugging the startup script. Left here for reference.
# resource local_file "startup_script" {
#   content  = local.startup_script
#   filename = "${path.module}/startup_script.sh"
# }

# Create a VM where the workspace will run.
resource "google_compute_instance" "vm" {
  name         = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}-root"
  machine_type = data.coder_parameter.instance_type.value
  # data.coder_workspace_owner.me.name == "default"  is a workaround to suppress error in the terraform plan phase while creating a new workspace.
  desired_status = (data.coder_workspace_owner.me.name == "default" || data.coder_workspace.me.start_count == 1) ? "RUNNING" : "TERMINATED"

  network_interface {
    network = "default"
    access_config {
      // Ephemeral public IP
    }
  }

  boot_disk {
    auto_delete = false
    source      = google_compute_disk.root.name
  }

  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    # The startup script runs as root with no $HOME environment set up, so instead of directly
    # running the agent init script, create a user (with a homedir, default shell and sudo
    # permissions) and execute the init script as that user.
    startup-script = local.startup_script
  }
}

# Create a Coder agent to manage the workspace.
resource "coder_agent" "dev" {
  count              = data.coder_workspace.me.start_count
  arch               = "amd64"
  auth               = "token"
  os                 = "linux"
  dir                = "/workspaces/${trimsuffix(basename(data.coder_parameter.repo_url.value), ".git")}"
  connection_timeout = 0

  metadata {
    key          = "cpu"
    display_name = "CPU Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat cpu"
  }
  metadata {
    key          = "memory"
    display_name = "Memory Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat mem"
  }
  metadata {
    key          = "disk"
    display_name = "Disk Usage"
    interval     = 5
    timeout      = 5
    script       = "coder stat disk"
  }
}

# See https://registry.coder.com/modules/code-server
module "code-server" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/code-server/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id = coder_agent.main.id
  order    = 1
}

# See https://registry.coder.com/modules/jetbrains-gateway
module "jetbrains_gateway" {
  count  = data.coder_workspace.me.start_count
  source = "registry.coder.com/modules/jetbrains-gateway/coder"

  # JetBrains IDEs to make available for the user to select
  jetbrains_ides = ["IU", "PY", "WS", "PS", "RD", "CL", "GO", "RM"]
  default        = "IU"

  # Default folder to open when starting a JetBrains IDE
  folder = "/home/coder"

  # This ensures that the latest version of the module gets downloaded, you can also pin the module version to prevent breaking changes in production.
  version = ">= 1.0.0"

  agent_id   = coder_agent.main.id
  agent_name = "main"
  order      = 2
}

# Create metadata for the workspace and home disk.
resource "coder_metadata" "workspace_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = google_compute_instance.vm.id

  item {
    key   = "type"
    value = google_compute_instance.vm.machine_type
  }

  item {
    key   = "zone"
    value = module.gcp_region.value
  }
}

resource "coder_metadata" "home_info" {
  resource_id = google_compute_disk.root.id

  item {
    key   = "size"
    value = "${google_compute_disk.root.size} GiB"
  }
}
