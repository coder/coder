# Advanced dev container configuration

This guide covers advanced configurations for [dev containers](./devcontainers.md) in [Coder templates](../index.md).

## Multiple dev containers

Run multiple dev containers in a single workspace for microservices or multi-component development:

```terraform
# Frontend dev container

resource "coder_devcontainer" "frontend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/frontend"
}

# Backend dev container

resource "coder_devcontainer" "backend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/backend"
}

# Database dev container

resource "coder_devcontainer" "database" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/database"
}

# Clone multiple repositories

module "git-clone-frontend" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.dev.id
  url      = "https://github.com/your-org/frontend.git"
  path     = "/home/coder/frontend"
}

module "git-clone-backend" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.dev.id
  url      = "https://github.com/your-org/backend.git"
  path     = "/home/coder/backend"
}
```

Each dev container will appear as a separate agent in the Coder UI, allowing developers to connect to different environments within the same workspace.

## Conditional startup with user control

Allow users to selectively enable dev containers:

```terraform
data "coder_parameter" "enable_frontend" {
  type        = "bool"
  name        = "Enable frontend dev container"
  default     = true
  description = "Start the frontend dev container automatically"
  mutable     = true
  order       = 3
}

data "coder_parameter" "enable_backend" {
  type        = "bool"
  name        = "Enable backend dev container"
  default     = true
  description = "Start the backend dev container automatically"
  mutable     = true
  order       = 4
}

resource "coder_devcontainer" "frontend" {
  count            = data.coder_parameter.enable_frontend.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/frontend"
}

resource "coder_devcontainer" "backend" {
  count            = data.coder_parameter.enable_backend.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/backend"
}
```

## Repository selection patterns

### Dropdown with predefined projects

```terraform
data "coder_parameter" "project" {
  name         = "Project"
  description  = "Select a project to work on"
  type         = "string"
  mutable      = true
  order        = 1

  option {
    name        = "E-commerce Frontend"
    description = "React-based e-commerce frontend"
    value       = "https://github.com/your-org/ecommerce-frontend.git"
    icon        = "/icon/react.svg"
  }

  option {
    name        = "Payment Service"
    description = "Node.js payment processing service"
    value       = "https://github.com/your-org/payment-service.git"
    icon        = "/icon/nodejs.svg"
  }

  option {
    name        = "User Management API"
    description = "Python user management API"
    value       = "https://github.com/your-org/user-api.git"
    icon        = "/icon/python.svg"
  }
}

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.dev.id
  url      = data.coder_parameter.project.value
  path     = "/home/coder/project"
}
```

### Team-based repository access

```terraform
data "coder_parameter" "team" {
  name         = "Team"
  description  = "Select your team"
  type         = "string"
  mutable      = true
  order        = 1

  option {
    name  = "Frontend Team"
    value = "frontend"
    icon  = "/icon/frontend.svg"
  }

  option {
    name  = "Backend Team"
    value = "backend"
    icon  = "/icon/backend.svg"
  }

  option {
    name  = "DevOps Team"
    value = "devops"
    icon  = "/icon/devops.svg"
  }
}

locals {
  team_repos = {
    frontend = [
      "https://github.com/your-org/web-app.git",
      "https://github.com/your-org/mobile-app.git"
    ]
    backend = [
      "https://github.com/your-org/api-service.git",
      "https://github.com/your-org/auth-service.git"
    ]
    devops = [
      "https://github.com/your-org/infrastructure.git",
      "https://github.com/your-org/monitoring.git"
    ]
  }
}

module "git-clone-team-repos" {
  count    = length(local.team_repos[data.coder_parameter.team.value]) * data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.dev.id
  url      = local.team_repos[data.coder_parameter.team.value][count.index % length(local.team_repos[data.coder_parameter.team.value])]
  path     = "/home/coder/repos/$sename(local.team_repos[data.coder_parameter.team.value][count.index % length(local.team_repos[data.coder_parameter.team.value])])}"
}
```

## Advanced infrastructure configurations

### Resource limits and monitoring

```terraform
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count

# ... other configuration

# Set resource limits

  memory = 4096  # 4GB
  cpus   = 2.0   # 2 CPU cores

# Enable swap accounting

  memory_swap = 8192  # 8GB including swap
}

resource "coder_agent" "dev" {

# ... other configuration

# Monitor dev container status

  metadata {
    display_name = "Dev Containers"
    key          = "devcontainers"
    script       = "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep -v NAMES || echo 'No dev containers running'"
    interval     = 30
    timeout      = 5
  }

  metadata {
    display_name = "Docker Resource Usage"
    key          = "docker_resources"
    script       = "docker stats --no-stream --format 'table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}' | head -5"
    interval     = 60
    timeout      = 10
  }
}
```

### Custom Docker networks

```terraform
resource "docker_network" "dev_network" {
  name = "coder-$ta.coder_workspace.me.id}-dev"
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count

# ... other configuration

  networks_advanced {
    name = docker_network.dev_network.name
  }
}
```

### Volume management for performance

```terraform
resource "docker_volume" "node_modules" {
  name = "coder-$ta.coder_workspace.me.id}-node-modules"
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_volume" "go_cache" {
  name = "coder-$ta.coder_workspace.me.id}-go-cache"
  lifecycle {
    ignore_changes = all
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count

# ... other configuration

# Persist node_modules for faster installs

  volumes {
    container_path = "/home/coder/project/node_modules"
    volume_name    = docker_volume.node_modules.name
  }

# Persist Go module cache

  volumes {
    container_path = "/home/coder/go/pkg/mod"
    volume_name    = docker_volume.go_cache.name
  }
}
```

## Dev container template example

You can test the Coder dev container integration and features with this example template.

Example template infrastructure requirements:

- Docker host with Docker daemon and socket available at `/var/run/docker.sock`.
- Network access to pull `codercom/enterprise-base:ubuntu` image.
- Access to Coder registry modules at `dev.registry.coder.com` and `registry.coder.com`.

Customization needed:

- Replace the default repository URL with your preferred repository.
- Adjust volume paths if your setup differs from `/home/coder`.
- Modify resource limits if needed for your workloads.

<details><summary>Expand for the example template:</summary>

```terraform
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 2.5"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

provider "coder" {}
provider "docker" {}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}
data "coder_provisioner" "me" {}

data "coder_parameter" "repo_url" {
  name         = "Repository URL"
  description  = "Git repository with devcontainer.json"
  type         = "string"
  default      = "https://github.com/microsoft/vscode-remote-try-node.git"
  mutable      = true
  order        = 1
}

data "coder_parameter" "enable_devcontainer" {
  type        = "bool"
  name        = "Enable dev container"
  default     = true
  description = "Automatically start the dev container"
  mutable     = true
  order       = 2
}

# Create persistent volume for home directory

resource "docker_volume" "home_volume" {
  name = "coder-$ta.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
}

# Main workspace container

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  name  = "coder-$ta.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"

# Hostname makes the shell more user friendly

  hostname = data.coder_workspace.me.name

# Required environment variables

  env = [
    "CODER_AGENT_TOKEN=${coder_agent.dev.token}",
    "CODER_AGENT_DEVCONTAINERS_ENABLE=true",
  ]

# Mount home directory for persistence

  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

# Mount Docker socket for dev container support

  volumes {
    container_path = "/var/run/docker.sock"
    host_path      = "/var/run/docker.sock"
    read_only      = false
  }

# Use the Docker host gateway for Coder access

  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
}

# Coder agent

resource "coder_agent" "dev" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
  dir  = "/home/coder"

  startup_script_behavior = "blocking"
  startup_script = <<-EOT
    set -e

    # Start Docker service
    sudo service docker start

    # Wait for Docker to be ready
    timeout 60 bash -c 'until docker info >/dev/null 2>&1; do sleep 1; done'

    echo "Workspace ready!"
  EOT

  shutdown_script = <<-EOT
    sudo service docker stop
  EOT

# Git configuration

  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = data.coder_workspace_owner.me.email
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = data.coder_workspace_owner.me.email
  }
}

# Install devcontainers CLI

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}

# Clone repository

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.dev.id
  url      = data.coder_parameter.repo_url.value
  path     = "/home/coder/project"
}

# Auto-start dev container

resource "coder_devcontainer" "project" {
  count            = data.coder_parameter.enable_devcontainer.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.dev.id
  workspace_folder = "/home/coder/project"
}

# Add code-server for web-based development

module "code-server" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.dev.id
  order    = 1
}
```

</details>

## Troubleshooting and debugging

You can troubleshoot dev container issues within a template or use the
[troubleshooting dev containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
documentation for issues that affect dev containers within user workspaces.

### Debug startup issues

```terraform
resource "coder_agent" "dev" {

# ... other configuration

  startup_script = <<-EOT
    set -e

    # Enable debug logging
    exec > >(tee -a /tmp/startup.log) 2>&1
    echo "=== Startup Debug Log $(date) ==="

    # Start Docker service with verbose logging
    sudo service docker start

    # Wait for Docker and log status
    echo "Waiting for Docker daemon..."
    timeout 60 bash -c 'until docker info >/dev/null 2>&1; do
      echo "Docker not ready, waiting..."
      sleep 2
    done'

    echo "Docker daemon ready!"
    docker version

    # Log dev container status
    echo "=== Dev Container Status ==="
    docker ps -a

    echo "Workspace startup complete!"
  EOT
}
```

### Add health checks and monitoring

```terraform
resource "coder_agent" "dev" {

# ... other configuration

  metadata {
    display_name = "Docker Service Status"
    key          = "docker_status"
    script       = "systemctl is-active docker || echo 'Docker service not running'"
    interval     = 30
    timeout      = 5
  }

  metadata {
    display_name = "Dev Container Health"
    key          = "devcontainer_health"
    script       = <<-EOT
      containers=$(docker ps --filter "label=devcontainer.local_folder" --format "{{.Names}}")
      if [ -z "$containers" ]; then
        echo "No dev containers running"
      else
        echo "Running: $containers"
      fi
    EOT
    interval     = 60
    timeout      = 10
  }
}
```

### Dev containers fail to start

1. Check Docker service status:

   ```shell
   systemctl status docker
   ```

1. Verify Docker socket permissions.
1. Review startup logs in `/tmp/startup.log`.

### Poor performance with multiple containers

1. Implement volume caching for package managers.
1. Set appropriate resource limits.
1. Use Docker networks for container communication.

### Repository access problems

1. Verify Git credentials are configured.
1. Check network connectivity to repository hosts.
1. Ensure repository URLs are accessible from the workspace.
