terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    aws = {
      source = "hashicorp/aws"
    }
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

module "aws_region" {
  source  = "https://registry.coder.com/modules/aws-region"
  default = "us-east-1"
}

provider "aws" {
  region = module.aws_region.value
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

variable "iam_instance_profile" {
  default     = ""
  description = "(Optional) Name of an IAM instance profile to assign to the instance."
  type        = string
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
  owners = ["099720109477"] # Canonical
}

data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance type"
  description  = "What instance type should your workspace use?"
  default      = "t3.micro"
  mutable      = false
  option {
    name  = "2 vCPU, 1 GiB RAM"
    value = "t3.micro"
  }
  option {
    name  = "2 vCPU, 2 GiB RAM"
    value = "t3.small"
  }
  option {
    name  = "2 vCPU, 4 GiB RAM"
    value = "t3.medium"
  }
  option {
    name  = "2 vCPU, 8 GiB RAM"
    value = "t3.large"
  }
  option {
    name  = "4 vCPU, 16 GiB RAM"
    value = "t3.xlarge"
  }
  option {
    name  = "8 vCPU, 32 GiB RAM"
    value = "t3.2xlarge"
  }
}

data "coder_parameter" "root_volume_size_gb" {
  name         = "root_volume_size_gb"
  display_name = "Root Volume Size (GB)"
  description  = "How large should the root volume for the instance be?"
  default      = 30
  type         = "number"
  mutable      = true
  validation {
    min       = 1
    monotonic = "increasing"
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

data "coder_parameter" "ssh_pubkey" {
  name         = "ssh_pubkey"
  display_name = "SSH Public Key"
  default      = ""
  description  = "(Optional) Add an SSH public key to the `coder` user's authorized_keys. Useful for troubleshooting. You may need to add a security group to the instance."
  mutable      = false
}

data "local_sensitive_file" "cache_repo_dockerconfigjson" {
  count    = var.cache_repo_docker_config_path == "" ? 0 : 1
  filename = var.cache_repo_docker_config_path
}

data "aws_iam_instance_profile" "vm_instance_profile" {
  count = var.iam_instance_profile == "" ? 0 : 1
  name  = var.iam_instance_profile
}

# Be careful when modifying the below locals!
locals {
  # TODO: provide a way to pick the availability zone.
  aws_availability_zone = "${module.aws_region.value}a"
  linux_user            = "coder"
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
  # User data to start the workspace.
  user_data = <<-EOT
  Content-Type: multipart/mixed; boundary="//"
  MIME-Version: 1.0

  --//
  Content-Type: text/cloud-config; charset="us-ascii"
  MIME-Version: 1.0
  Content-Transfer-Encoding: 7bit
  Content-Disposition: attachment; filename="cloud-config.txt"

  #cloud-config
  cloud_final_modules:
  - [scripts-user, always]
  hostname: ${lower(data.coder_workspace.me.name)}
  users:
  - name: ${local.linux_user}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
    - "${data.coder_parameter.ssh_pubkey.value}"
  # Automatically grow the partition
  growpart:
    mode: auto
    devices: ['/']
    ignore_growroot_disabled: false

  --//
  Content-Type: text/x-shellscript; charset="us-ascii"
  MIME-Version: 1.0
  Content-Transfer-Encoding: 7bit
  Content-Disposition: attachment; filename="userdata.txt"

  #!/bin/bash
  # Install Docker
  if ! command -v docker &> /dev/null
  then
    echo "Docker not found, installing..."
    curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh 2>&1 >/dev/null
    usermod -aG docker ${local.linux_user}
    newgrp docker
  else
    echo "Docker is already installed."
  fi

  # Set up Docker credentials
  mkdir -p "/home/${local.linux_user}/.docker"
  if [ -n "${local.docker_config_json_base64}" ]; then
     # Write the Docker config JSON to disk if it is provided.
     printf "%s" "${local.docker_config_json_base64}" | base64 -d | tee "/home/${local.linux_user}/.docker/config.json"
  else
    # Assume that we're going to use the instance IAM role to pull from the cache repo if we need to.
    # Set up the ecr credential helper.
    apt-get update -y && apt-get install -y amazon-ecr-credential-helper
    mkdir -p .docker
    printf '{"credsStore": "ecr-login"}' | tee "/home/${local.linux_user}/.docker/config.json"
  fi
  chown -R ${local.linux_user}:${local.linux_user} "/home/${local.linux_user}/.docker"

  # Write the container env to disk.
  printf "%s" "${local.docker_env_list_base64}" | base64 -d | tee "/home/${local.linux_user}/env.txt"

  # Start envbuilder
  sudo -u coder docker run \
    --rm \
    --net=host \
    -h ${lower(data.coder_workspace.me.name)} \
    -v /home/${local.linux_user}/envbuilder:/workspaces \
    -v /var/run/docker.sock:/var/run/docker.sock \
    --env-file /home/${local.linux_user}/env.txt \
    ${local.builder_image}
  --//--
  EOT
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
#   content  = local.user_data
#   filename = "${path.module}/user_data.txt"
# }

resource "aws_instance" "vm" {
  ami                  = data.aws_ami.ubuntu.id
  availability_zone    = local.aws_availability_zone
  instance_type        = data.coder_parameter.instance_type.value
  iam_instance_profile = try(data.aws_iam_instance_profile.vm_instance_profile[0].name, null)
  root_block_device {
    volume_size = data.coder_parameter.root_volume_size_gb.value
  }

  user_data = local.user_data
  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${data.coder_workspace.me.name}"
    # Required if you are using our example policy, see template README
    Coder_Provisioned = "true"
  }
  lifecycle {
    ignore_changes = [ami]
  }
}

resource "aws_ec2_instance_state" "vm" {
  instance_id = aws_instance.vm.id
  state       = data.coder_workspace.me.transition == "start" ? "running" : "stopped"
}

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
}

resource "coder_metadata" "info" {
  count       = data.coder_workspace.me.start_count
  resource_id = coder_agent.dev[0].id
  item {
    key   = "ami"
    value = aws_instance.vm.ami
  }
  item {
    key   = "availability_zone"
    value = local.aws_availability_zone
  }
  item {
    key   = "instance_type"
    value = data.coder_parameter.instance_type.value
  }
  item {
    key   = "ssh_pubkey"
    value = data.coder_parameter.ssh_pubkey.value
  }
  item {
    key   = "repo_url"
    value = data.coder_parameter.repo_url.value
  }
  item {
    key   = "devcontainer_builder"
    value = data.coder_parameter.devcontainer_builder.value
  }
}

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.dev[0].id
}
