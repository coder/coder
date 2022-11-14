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
coder templates create <template-name>
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
template options, reference [Coder Terraform provider docs](https://registry.terraform.io/providers/coder/coder/latest/docs).

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

The `coder_agent` resource can be configured as described in the
[documentation for the `coder` Terraform provider.](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent)
For example, you can use the `env` property to set environment variables that will be
inherited by all child processes of the agent, including SSH sessions.

#### startup_script

Use the Coder agent's `startup_script` to run additional commands like
installing IDEs, [cloning dotfiles](./dotfiles.md#templates), and cloning project repos.

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir = "/home/coder"
  startup_script = <<EOT
#!/bin/bash

# install code-server
curl -fsSL https://code-server.dev/install.sh | sh

# The & prevents the startup_script from blocking so the
# next commands can run.
code-server --auth none --port &

# var.repo and var.dotfiles_uri is specified
# elsewhere in the Terraform code as input
# variables.

# clone repo
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts
git clone --progress git@github.com:${var.repo}

# use coder CLI to clone and install dotfiles
coder dotfiles -y ${var.dotfiles_uri}

  EOT
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

The template sample below uses _admin and user parameters_ to allow developers to
create workspaces from any image as long as it is in the proper registry:

```hcl
variable "image_registry_url" {
  description = "The image registry developers can select"
  default     = "artifactory1.organization.com"
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

#### Start/stop

[Learn about resource persistence in Coder](./templates/resource-persistence.md)

Coder workspaces can be started/stopped. This is often used to save on cloud costs or enforce
ephemeral workflows. When a workspace is started or stopped, the Coder server
runs an additional
[terraform apply](https://www.terraform.io/cli/commands/apply), informing the
Coder provider that the workspace has a new transition state.

This template sample has one persistent resource (docker volume) and one ephemeral resource
(docker image).

```hcl
data "coder_workspace" "me" {
}

resource "docker_volume" "home_volume" {
  # persistent resource (remains a workspace is stopped)
  count = 1
  name  = "coder-${data.coder_workspace.me.id}-home"
  lifecycle {
    ignore_changes = all
  }
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

#### Using updated images when rebuilding a workspace

To ensure that Coder uses an updated image when rebuilding a workspace, we
suggest that admins update the tag in the template (e.g., `my-image:v0.4.2` ->
`my-image:v0.4.3`) or digest (`my-image@sha256:[digest]` ->
`my-image@sha256:[new_digest]`).

Alternatively, if you're willing to wait for longer start times from Coder, you
can set the `imagePullPolicy` to `Always` in your Terraform template; when set,
Coder will check `image:tag` on every build and update if necessary:

```tf
resource "kubernetes_pod" "podName" {
    spec {
        container {
            image_pull_policy = "Always"
        }
    }
}
```

### Edit templates

You can edit a template using the coder CLI. Only
[template admins and owners](./admin/users.md) can edit a template.

Using the CLI, login to Coder and run the following command to edit a single template:

```sh
coder templates edit <template-name> --description "This is my template"
```

Review editable template properties by running `coder templates edit -h`.

Alternatively, you can pull down the template as a tape archive (`.tar`) to your current directory:

```sh
coder templates pull <template-name> file.tar
```

Then, extract it by running:

```sh
tar -xf file.tar
```

Make the changes to your template then run this command from the root of the template folder:

```sh
coder templates push <template-name>
```

Your updated template will now be available. Outdated workspaces will have a prompt in the dashboard to update.

### Delete templates

You can delete a template using both the coder CLI and UI. Only
[template admins and owners](./admin/users.md) can delete a template, and the template
must not have any running workspaces associated to it.

Using the CLI, login to Coder and run the following command to delete a template:

```console
coder templates delete <template-name>
```

In the UI, navigate to the template you want to delete, and select the dropdown in
the right-hand corner of the page to delete the template.

![delete-template](./images/delete-template.png)

#### Delete workspaces

When a workspace is deleted, the Coder server essentially runs a
[terraform destroy](https://www.terraform.io/cli/commands/destroy) to remove all
resources associated with the workspace.

> Terraform's
> [prevent-destroy](https://www.terraform.io/language/meta-arguments/lifecycle#prevent_destroy)
> and
> [ignore-changes](https://www.terraform.io/language/meta-arguments/lifecycle#ignore_changes)
> meta-arguments can be used to prevent accidental data loss.

### Coder apps

By default, all templates allow developers to connect over SSH and a web
terminal. See [Configuring Web IDEs](./ides/web-ides.md) to
learn how to give users access to additional web applications.

### Data source

When a workspace is being started or stopped, the `coder_workspace` data source provides
some useful parameters. See the [documentation for the `coder` Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace)
for more information.

For example, the [Docker quick-start template](https://github.com/coder/coder/tree/main/examples/templates/docker)
sets a few environment variables based on the username and email address of the workspace's owner, so
that you can make Git commits immediately without any manual configuration:

```tf
resource "coder_agent" "main" {
  # ...
  env = {
    GIT_AUTHOR_NAME = "${data.coder_workspace.me.owner}"
    GIT_COMMITTER_NAME = "${data.coder_workspace.me.owner}"
    GIT_AUTHOR_EMAIL = "${data.coder_workspace.me.owner_email}"
    GIT_COMMITTER_EMAIL = "${data.coder_workspace.me.owner_email}"
  }
}
```

You can add these environment variable definitions to your own templates, or customize them however
you like.

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
  - The Coder agent logs are typically stored in `/var/log/coder-agent.log`
  - The Coder agent startup script logs are typically stored in `/var/log/coder-startup-script.log`

## Template permissions (enterprise)

Template permissions can be used to give users and groups access to specific templates. [Learn more about RBAC](./admin/rbac.md).

## Next Steps

- Learn about [Authentication & Secrets](templates/authentication.md)
- Learn about [Change Management](templates/change-management.md)
- Learn about [Resource Metadata](templates/resource-metadata.md)
- Learn about [Workspaces](workspaces.md)
