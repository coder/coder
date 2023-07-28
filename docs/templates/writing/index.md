# Writing Coder templates

Coder templates are written in [Terraform](https://terraform.io). All Terraform modules, resources, and properties can be provisioned by Coder. The Coder server essentially runs a `terraform apply` every time a workspace is created/started/stopped.

Haven't written Terraform before? Check out Hashicorp's [Getting Started Guides](https://developer.hashicorp.com/terraform/tutorials).

## Key concepts in templates

There are some key concepts you should consider when writing templates.

## Coder Terraform Provider

The [Coder Terraform provider](https://registry.terraform.io/providers/coder/coder/latest) makes it possible for standard Terraform resources (e.g. `docker_container`) to connect to Coder. Additionally, the provider lets you to customize the behavior of workspaces using your template.

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
    }
  }
}
```

### coder_workspace

This data source provides details about the state of a workspace, such as its name, owner, and whether the workspace is being started or stopped.

The following snippet will create a container when the workspace is being started, and delete the container when it is stopped using Terraform's [count](https://developer.hashicorp.com/terraform/language/meta-arguments/count) meta-argument.

```hcl
data "coder_workspace" "me" {}

# Delete the container when workspace is stopped (count = 0)
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.transition == "start" ? 1 : 0
  # ...
}

# Persist the volume, even if stopped
resource "docker_volume" "projects" {}
```

### coder_agent

All templates need to create & run a Coder agent in order for developers to connect to their workspaces. The Coder agent is a service that runs inside the compute aspect of your workspace (typically a VM or container).

This snippet creates the agent, runs it inside the container via the `entrypoint`, and authenticates to Coder via the agent's token.

```hcl
resource "coder_agent" "main" {
  os = "linux"
  arch = "amd64"
}

resource "docker_container" "workspace" {
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  # ...
}
```

Agents can also run startup scripts, set environment variables, and provide metadata about the workspace (e.g. CPU usage). Read the [coder_agent docs](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script) for more details.
