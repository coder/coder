# Advanced Dev Container Configuration

This page extends [devcontainers.md](./devcontainers.md) with patterns for multiple dev containers,
user-controlled startup, repository selection, and infrastructure tuning.

## Run Multiple Dev Containers

Run independent dev containers in the same workspace so each component appears as its own agent.

In this example, there are three: `frontend`, `backend`, and a `database`:

```terraform
# Clone each repo
module "git_clone_frontend" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.main.id
  url      = "https://github.com/your-org/frontend.git"
  base_dir = "/home/coder/frontend"
}

module "git_clone_backend" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.main.id
  url      = "https://github.com/your-org/backend.git"
  base_dir = "/home/coder/backend"
}

module "git_clone_database" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.main.id
  url      = "https://github.com/your-org/database.git"
  base_dir = "/home/coder/database"
}

# Dev container resources
resource "coder_devcontainer" "frontend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/frontend/${module.git_clone_frontend[0].folder_name}"
  depends_on       = [module.git_clone_frontend]
}

resource "coder_devcontainer" "backend" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/backend/${module.git_clone_backend[0].folder_name}"
  depends_on       = [module.git_clone_backend]
}

resource "coder_devcontainer" "database" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/database/${module.git_clone_database[0].folder_name}"
  depends_on       = [module.git_clone_database]
}
```

Each dev container appears as a separate agent, so developers can connect to any
component in the workspace.

## Personal Overrides

Let developers extend the repo’s `devcontainer.json` with an ignored (by Git) `devcontainer.local.json` file
so they can add personal tools without changing the canonical configuration:

```jsonc
{
  "extends": "./devcontainer.json",
  "features": {
    "ghcr.io/devcontainers/features/node": { "version": "20" }
  },
  "postStartCommand": "npm i -g tldr"
}
```

Add the file name to your project's `.gitignore` or the user's
[global exclude file](https://docs.github.com/en/get-started/git-basics/ignoring-files#configuring-ignored-files-for-all-repositories-on-your-computer).

## Conditional Startup

Use `coder_parameter` booleans to let workspace creators choose which dev containers start automatically,
reducing resource usage for unneeded components:

```terraform
data "coder_parameter" "enable_frontend" {
  type        = "bool"
  name        = "Enable frontend container"
  default     = true
  mutable     = true
  order       = 3
}

resource "coder_devcontainer" "frontend" {
  count            = data.coder_parameter.enable_frontend.value ? data.coder_workspace.me.start_count : 0
  agent_id         = coder_agent.main.id
  workspace_folder = "/home/coder/frontend/${module.git_clone_frontend[0].folder_name}"
  depends_on       = [module.git_clone_frontend]
}
```

## Repository-selection Patterns

Prompt users to pick a repository or team at workspace creation time and clone the selected repo(s) automatically into the workspace:

### Dropdown selector

```terraform
data "coder_parameter" "project" {
  name        = "Project"
  description = "Choose a project"
  type        = "string"
  mutable     = true
  order       = 1

  option { name = "E-commerce FE" value = "https://github.com/org/ecom-fe.git" icon = "/icon/react.svg" }
  option { name = "Payment API"  value = "https://github.com/org/pay.git"      icon = "/icon/nodejs.svg" }
}

module "git_clone_selected" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.main.id
  url      = data.coder_parameter.project.value
  base_dir = "/home/coder/project"
}
```

### Team-based selection

```terraform
data "coder_parameter" "team" {
  name    = "Team"
  type    = "string"
  mutable = true
  order   = 1

  option { name = "Frontend" value = "frontend" icon = "/icon/react.svg" }
  option { name = "Backend"  value = "backend"  icon = "/icon/nodejs.svg" }
}

locals {
  repos = {
    frontend = ["https://github.com/your-org/web.git"]
    backend  = ["https://github.com/your-org/api.git"]
  }
}

module "git_clone_team" {
  count    = length(local.repos[data.coder_parameter.team.value]) * data.coder_workspace.me.start_count
  source   = "registry.coder.com/modules/git-clone/coder"
  version  = "~> 1.0"

  agent_id = coder_agent.main.id
  url      = local.repos[data.coder_parameter.team.value][count.index]
  base_dir = "/home/coder/${replace(basename(url), \".git\", \"\")}"
}
```

## Infrastructure Tuning

Adjust workspace infrastructure to set memory/CPU limits, attach a custom Docker network,
or add persistent volumes—to improve performance and isolation for dev containers:

### Resource limits

```terraform
resource "docker_container" "workspace" {
  count    = data.coder_workspace.me.start_count
  image    = "codercom/enterprise-node:ubuntu"

  resources {
    memory = 4096   # MiB
    cpus   = 2
    memory_swap = 8192
  }
}
```

### Custom network

```terraform
resource "docker_network" "dev" {
  name = "coder-${data.coder_workspace.me.id}-dev"
}

resource "docker_container" "workspace" {
  networks_advanced { name = docker_network.dev.name }
}
```

### Volume caching

```terraform
resource "docker_volume" "node_modules" {
  name = "coder-${data.coder_workspace.me.id}-node-modules"
  lifecycle { ignore_changes = all }
}

resource "docker_container" "workspace" {
  volumes {
    container_path = "/home/coder/project/node_modules"
    volume_name    = docker_volume.node_modules.name
  }
}
```

## Troubleshooting

1. Run `docker ps` inside the workspace to ensure Docker is available.
1. Check `/tmp/startup.log` for agent logs.
1. Verify the workspace image includes Node/npm or add the `nodejs` module before the `devcontainers_cli` module.
