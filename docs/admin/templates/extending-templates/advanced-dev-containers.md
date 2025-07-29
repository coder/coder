# Advanced Dev Container Configuration

This page extends the [dev containers documentation](./devcontainers.md) with patterns for multiple dev containers,
user-controlled startup, repository selection, and infrastructure tuning.

## Run Multiple Dev Containers

Run independent dev containers in the same workspace so each component appears as its own agent.

In this example, there are two: repositories called `frontend` and `backend`:

```terraform
# Clone each repo
module "git-clone-frontend" {
  count  = data.coder_workspace.me.start_count
  source  = "registry.coder.com/modules/git-clone/coder"

  agent_id  = coder_agent.main.id
  url  = "https://github.com/your-org/frontend.git"
}

module "git-clone-backend" {
  count  = data.coder_workspace.me.start_count
  source  = "registry.coder.com/modules/git-clone/coder"

  agent_id  = coder_agent.main.id
  url  = "https://github.com/your-org/backend.git"
}

# Dev container resources
resource "coder_devcontainer" "frontend" {
  count  = data.coder_workspace.me.start_count
  agent_id  = coder_agent.main.id
  workspace_folder  = module.git-clone-frontend[0].repo_dir
}

resource "coder_devcontainer" "backend" {
  count  = data.coder_workspace.me.start_count
  agent_id  = coder_agent.main.id
  workspace_folder  = module.git-clone-backend[0].repo_dir
}
```

Each dev container appears as a separate agent, so developers can connect to each separately.

## Conditional Startup

Use `coder_parameter` booleans to let workspace creators choose which dev containers start automatically,
reducing resource usage for unneeded components:

```terraform
data "coder_parameter" "autostart_frontend" {
  type  = "bool"
  name  = "Autostart frontend container"
  default  = true
  mutable  = true
  order  = 3
}

resource "coder_devcontainer" "frontend" {
  count  = data.coder_parameter.autostart_frontend.value ? data.coder_workspace.me.start_count : 0
  agent_id  = coder_agent.main.id
  workspace_folder  = module.git-clone-frontend[0].repo_dir
}
```

## Repository-selection Patterns

Prompt users to pick a repository or team at workspace creation time and clone the selected repo(s) automatically into the workspace:

1. Add a parameter to the template:

   ```terraform
   data "coder_parameter" "project" {
     name  = "project"
     display_name  = "Choose a project"
     type  = "string"
     default  = "https://github.com/coder/coder.git"

     option {
        name  = "coder/coder"
        value  = "https://github.com/coder/coder.git"
     }
     option {
       name  = "terraform-provider-coder"
       value  = "https://github.com/coder/terraform-provider-coder.git"
     }
   }
   ```

1. Change the `git-clone` module to accept the `value` as the `url`:

    ```terraform
    module "git-clone" {
     count  = data.coder_workspace.me.start_count
     source  = "registry.coder.com/modules/git-clone/coder"
     agent_id  = coder_agent.main.id
     url  = data.coder_parameter.project.value
    }
    ```
