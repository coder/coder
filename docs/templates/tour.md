# A guided tour of a template

This guided tour introduces you to the different parts of a template
by showing you how to create a template from scratch.

In this tour you'll write a simple template that starts a Docker
container with Ubuntu. This simple template is based on the same
Docker starter template that the [tutorial](./tutorial.md) uses.

## Before you start

To follow this guide, you'll need:

- A computer or cloud computing instance with both
[Docker](https://docs.docker.com/get-docker/) and
[Coder](../install/index.md) installed on it.

> When setting up your computer or computing instance, make sure to
> install Docker first, then Coder.

- Access to the command-line on this computer or instance.

- A text editor and a tar utility. This tour uses [GNU
nano](https://nano-editor.org/) and [GNU
tar](https://www.gnu.org/software/tar/).

> Haven't written Terraform before? Check out Hashicorp's [Getting Started Guides](https://developer.hashicorp.com/terraform/tutorials).

## What's in a template

The main part of a Coder template is a
[Terraform](https://terraform.io) `tf` file. A template often has
other files to configure other services that the template needs. In
this tour you'll also create a Dockerfile.

Coder can provision all Terraform modules, resources, and
properties. The Coder server essentially runs a `terraform apply`
every time a workspace is created, started, or stopped.

This is a simplified diagram of our [Kubernetes starter
template](https://github.com/coder/coder/blob/main/examples/templates/kubernetes/main.tf):

![Template architecture](../images/templates/template-anatomy.png)


## 1. Create template files

On the command line, create a directory for your template and create the Dockerfile.

This is a simple Dockerfile that starts with the [official ubuntu image](https://hub.docker.com/_/ubuntu/).

```shell
$ mkdir scratch-template
$ cd scratch-template
$ touch Dockerfile main.tf
$ nano Dockerfile
```

In the editor, enter and save the following text in `Dockerfile` then
exit the editor:

```
FROM ubuntu

RUN apt-get update \
	&& apt-get install -y \
	sudo \
	curl \
	&& rm -rf /var/lib/apt/lists/*

ARG USER=coder
RUN useradd --groups sudo --no-create-home --shell /bin/bash ${USER} \
	&& echo "${USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/${USER} \
	&& chmod 0440 /etc/sudoers.d/${USER}
USER ${USER}
WORKDIR /home/${USER}
```

Notice how `Dockerfile` adds a few things to the parent `ubuntu`
image, which we'll refer to later:

- It installs the `sudo` and `curl` packages.
- It adds a `coder` user, including a home directory.


## 2. Specify providers

Now you can edit the Terraform file, which provisions the workspace's resources.

```shell
nano main.tf
```

A Terraform file starts with The Terraform file starts with the
`terraform` block, which specifies providers. At a minimum, we need
the `coder` provider. For this template, we also need the `docker`
provider:

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.8.3"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.1"
    }
  }
}
```

## 3. coder_agent

All templates need to create and run a Coder agent to let developers
connect to their workspaces. The `coder_agent` resource runs inside
the compute aspect of your workspace (typically a VM or
container). You do not need to have any open ports on the compute
aspect, but the agent needs `curl` access to the Coder server.

This snippet creates the agent, runs it inside the container via the
`entrypoint`, and authenticates to Coder via the agent's token.

```hcl
resource "coder_agent" "main" {
  os = "linux"
  arch = "amd64"
}

resource "kubernetes_deployment" "workspace" {
  entrypoint = ["sh", "-c", coder_agent.main.init_script]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  # ...
}
```

Agents can also run startup scripts, set environment variables, and
provide [metadata](../agent-metadata.md) about the workspace (e.g. CPU
usage). See [coder_agent
docs](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script)
for more details.

## 3. coder_workspace

This data source provides details about the state of a workspace, such
as its name, owner, and whether the workspace is being started or
stopped.

The following snippet creates a container when the workspace is being
started, and deletes the container when it is stopped. It does this
with Terraform's
[count](https://developer.hashicorp.com/terraform/language/meta-arguments/count)
meta-argument.

```hcl
data "coder_workspace" "me" {}

# Delete the container when workspace is stopped (count = 0)
resource "kubernetes_deployment" "workspace" {
  count = data.coder_workspace.me.transition == "start" ? 1 : 0
  # ...
}

# Persist the volume, even if stopped
resource "docker_volume" "projects" {}
```

## 4. coder_app

Web apps that are running inside the workspace
(e.g. `http://localhost:8080`) can be forwarded to the Coder dashboard
with the `coder_app` resource. This is commonly used for [web
IDEs](../ides/web-ides.md) such as code-server, RStudio, and
JupyterLab. External apps, such as links to internal wikis or cloud
consoles can also be embedded here.

Apps are rendered on the workspace page:

![Apps in a Coder workspace](../images/templates/workspace-apps.png)

The apps themselves have to be installed and running on the
workspace. You can do this in the agent's `startup_script`. See [web
IDEs](../ides/web-ides.md) for some examples.

```hcl
# coder_agent will install and start code-server
resource "coder_agent" "main" {
  # ...
  startup_script =<<EOF
  curl -L https://code-server.dev/install.sh | sh
  code-server --port 8080 &
  EOF
}

# expose code-server on workspace via a coder_app
resource "coder_app" "code-server" {
  agent_id     = coder_agent.main.id
  icon         = "/icon/code.svg"
  display_name = "code-server"
  slug         = "code"
  url          = "http://localhost:8080"
}

# link to an external site
resource "coder_app" "getting-started" {
  agent_id     = coder_agent.main.id
  icon         = "/emojis/1f4dd.png"
  display_name = "getting-started"
  slug         = "getting-started"
  url          = "https://wiki.example.com/coder/quickstart"
  external     = true
}
```

## 5. Persistent storage

```hcl
resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace.me.owner
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace.me.owner_id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}
```

## Next steps

- [Setting up templates](./best-practices.md)
- [Customizing templates](./customizing.md)

