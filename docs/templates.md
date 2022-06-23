# Templates

Templates are written in standard Terraform and describe the infrastructure for
workspaces (e.g., aws_instance, kubernetes_pod, or both).

In most cases, a small group of users (Coder admins) manage templates. Then,
other users provision their development workspaces from templates.

## Add a template

Before users can create workspaces, you'll need at least one template in Coder.

```sh
# create a local directory to store templates
mkdir -p $HOME/coder/templates
cd $HOME/coder/templates

# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

> See the documentation and source code for each example in the
> [examples/](https://github.com/coder/coder/tree/main/examples/templates)
> directory in the repo.

## Customize templates

Example templates are not designed to support every use (e.g [examples/aws-linux](https://github.com/coder/coder/tree/main/examples/templates/aws-linux) does
not support custom VPCs). You can add these features by editing the Terraform
code once you run `coder templates init` (new) or `coder templates pull`
(existing).

- See [Creating and troubleshooting templates](#creating--troubleshooting-templates) for
  more info

## Concepts in templates

While templates are written with standard Terraform, the
[Coder Terraform Provider](https://registry.terraform.io/providers/coder/coder/latest/docs) is 
used to define the workspace lifecycle and establish a connection from resources
to Coder.

Below is an overview of some key concepts in templates (and workspaces). For all
template options, reference [Coder Terraform provider docs](https://registry.terraform.io/providers/kreuzwerker/docker/latest/docs/resources/container).

### Resource

Resources in Coder are simply [Terraform resources](https://www.terraform.io/language/resources). 
If a Coder agent is attached to a resource, users can connect directly to the resource over
SSH or web apps.

### Coder agent

Once a Coder workspace is created, the Coder agent establishes a connection
between a resource (docker_container) and Coder, so that a user can connect to
their workspace from the web UI or CLI. A template can have multiple agents to
allow users to connect to multiple resources in their workspace. 

> Resources must download and start the Coder agent binary to connect to Coder.
> This means the resource must be able to reach your Coder URL.

Use the Coder agent's init script to 

```hcl
data "coder_workspace" "me" {
}

resource "coder_agent" "pod1" {
  os   = "linux"
  arch = "amd64"
}

resource "kubernetes_pod" "pod1" {
  spec {
    ...
    container {
      command = ["sh", "-c", coder_agent.pod1.init_script]
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }
    }
  }
}
```

### Parameters

Templates often contain _parameters_. These are defined by `variable` blocks in
Terraform. There are two types of parameters:

- **Admin/template-wide parameters** are set when a template is created/updated. These values
  are often cloud configuration, such as a `VPC`, and are annotated
  with `sensitive = true` in the template code.
- **User/workspace parameters** are set when a user creates a workspace. These
  values are often personalization settings such as "preferred region"
  or "workspace image".

The template sample below uses *admin and user parameters* to allow developers to
create workspaces from any image as long as it is in the proper registry:

```hcl
variable "image_registry_url" {
  description = "The image registry developers can sele"
  default     = "artifactory1.organization.com`
  sensitive   = true # admin (template-wide) parameter
}

variable "docker_image_name" {
  description = "The image your workspace will start from"
  default     = "base_image"
  sensitive   = false # user (workspace) parameter
}

resource "docker_image" "workspace" {
  # ... other config
  name = "${var.image_registry_url}/${var.docker_image_name}"
}
```

### Persistent vs. ephemeral resources

You can use the workspace state to ensure some resources in Coder can are
persistent, while others are ephemeral.

#### Start/stop

Coder workspaces can be started/stopped. This is often used to save on cloud costs or enforce
ephemeral workflows. When a workspace is started or stopped, the Coder server
runs an additional
[terraform apply](https://www.terraform.io/cli/commands/apply), informing the
Coder provider that the workspace has a new transition state.

This template sample has one persistent resource (docker image) and one ephemeral resource
(docker volume).

```sh
data "coder_workspace" "me" {
}

resource "docker_volume" "home_volume" {
  # persistent resource (remains a workspace is stopped)
  count = 1
  name  = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}-root"
}

resource "docker_container" "workspace" {
  # ephemeral resource (deleted when workspace is stopped, created when started)
  count = data.coder_workspace.me.start_count # 0 (stopped), 1 (started)
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  # ... other config
}
```

#### Delete workspaces

When a workspace is deleted, the Coder server essentially runs a
[terraform destroy](https://www.terraform.io/cli/commands/destroy) to remove all
resources associated with the workspace.

> Terraform's
> [prevent-destroy](https://www.terraform.io/language/meta-arguments/lifecycle#prevent_destroy)
> and
> [ignore-changes](https://www.terraform.io/language/meta-arguments/lifecycle#ignore_changes)
> meta-arguments can be used to accidental data loss. 

### Coder apps

By default, all templates allow developers to connect over SSH and a web
terminal. See [Configuring Web IDEs](./ides/configuring-web-ides.md) to
learn how to give users access to additional web applications.

## Creating & troubleshooting templates

You can use any Terraform resources or modules with Coder! When working on
templates, we recommend you refer to the following resources:

- this document
- [example templates](https://github.com/coder/coder/tree/main/examples/templates) code
- [Coder Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs)
  documentation

Occasionally, you may run into scenarios where the agent is not able to connect.
This means the start script has failed.

```sh
$ coder ssh myworkspace
Waiting for [agent] to connect...
```

While troubleshooting steps vary by resource, here are some general best
practices:

- Ensure the resource has `curl` installed
- Ensure the resource can reach your Coder URL
- Manually connect to the resource (e.g., `docker exec` or AWS console)
  - The Coder agent logs are typically stored in (`/var/log/coder-agent.log`)


## Change Management

We recommend source controlling your templates as you would other code.

CI is as simple as running `coder templates update` with the appropriate
credentials.

---

Next: [Authentication & Secrets](templates/authentication.md) and [Workspaces](./workspaces.md)
