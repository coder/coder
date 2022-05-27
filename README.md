# Coder

[!["GitHub
Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions)
[!["Join us on
Discord"](https://img.shields.io/badge/join-us%20on%20Discord-gray.svg?longCache=true&logo=discord&colorB=purple)](https://discord.gg/coder)
[![Twitter
Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq)
[![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Coder creates remote development machines so you can develop your code from anywhere.

> **Note**:
> Coder is in an alpha state, but any serious bugs are P1 for us so please report them.

<p align="center">
  <img src="./docs/images/hero-image.png">
</p>

**Code more**

- Build and test faster
  - Leveraging cloud CPUs, RAM, network speeds, etc.
- Access your environment from any place on any client (even an iPad)
- Onboard instantly then stay up to date continuously

**Manage less**

- Ensure your entire team is using the same tools and resources
  - Rollout critical updates to your developers with one command
- Automatically shut down expensive cloud resources
- Keep your source code and data behind your firewall

## How it works

Coder workspaces are represented with Terraform. But, no Terraform knowledge is
required to get started. We have a database of pre-made templates built into the
product.

<p align="center">
  <img src="./docs/images/providers-compute.png">
</p>

Coder workspaces don't stop at compute. You can add storage buckets, secrets, sidecars
and whatever else Terraform lets you dream up.

[Learn more about managing infrastructure.](./docs/templates.md)

## IDE Support

You can use any Web IDE ([code-server](https://github.com/coder/code-server), [projector](https://github.com/JetBrains/projector-server), [Jupyter](https://jupyter.org/), etc.), [JetBrains Gateway](https://www.jetbrains.com/remote-development/gateway/), [VS Code Remote](https://code.visualstudio.com/docs/remote/ssh-tutorial) or even a file sync such as [mutagen](https://mutagen.io/).

<p align="center">
  <img src="./docs/images/ide-icons.svg" height=72>
</p>

## Installing Coder

We recommend installing [the latest
release](https://github.com/coder/coder/releases) on a system with at least 1
CPU core and 2 GB RAM:

1. Download the [release asset](https://github.com/coder/coder/releases) appropriate for your operating system
1. Unzip the folder you just downloaded, and move the `coder` executable to a
   location that's on your `PATH`

> Make sure you have the appropriate credentials for your cloud provider (e.g.,
> access key ID and secret access key for AWS).

You can set up a temporary deployment, a production deployment, or a system service:

- To set up a **temporary deployment**, start with dev mode (all data is in-memory and is
  destroyed on exit):

  ```bash
  coder server --dev
  ```

- To run a **production deployment** with PostgreSQL:

  ```bash
  CODER_PG_CONNECTION_URL="postgres://<username>@<host>/<database>?password=<password>" \
      coder server
  ```

- To run as a **system service**, install with `.deb` (Debian, Ubuntu) or `.rpm`
  (Fedora, CentOS, RHEL, SUSE):

  ```bash
  # Edit the configuration!
  sudo vim /etc/coder.d/coder.env
  sudo service coder restart
  ```

> Use `coder --help` to get a complete list of flags and environment
> variables.

See the [installation guide](./docs/install.md) for additional ways to deploy Coder.

## Creating your first template and workspace

In a new terminal window, run the following to copy a sample template:

```bash
coder templates init
```

Follow the CLI instructions to modify and create the template specific for your
usage (e.g., a template to **Develop in Linux on Google Cloud**).

Create a workspace using your template:

```bash
coder create --template="yourTemplate" <workspaceName>
```

Connect to your workspace via SSH:

```bash
coder ssh <workspaceName>
```

## Modifying templates

You can edit the Terraform template using a sample template:

```sh
coder templates init
cd gcp-linux/
vim main.tf
coder templates update gcp-linux
```

## Documentation

- [About Coder](./docs/about.md#about-coder)
  - [Why remote development](./docs/about.md#why-remote-development)
  - [Why Coder](./docs/about.md#why-coder)
  - [What Coder is not](./docs/about.md#what-coder-is-not)
  - [Comparison: Coder vs. [product]](./docs/about.md#comparison)
- [Templates](./docs/templates.md)
  - [Manage templates](./docs/templates.md#manage-templates)
  - [Persistent and ephemeral
    resources](./docs/templates.md#persistent-and-ephemeral-resources)
  - [Parameters](./docs/templates.md#parameters)
- [Workspaces](./docs/workspaces.md)
  - [Create workspaces](./docs/workspaces.md#create-workspaces)
  - [Connect with SSH](./docs/workspaces.md#connect-with-ssh)
  - [Editors and IDEs](./docs/workspaces.md#editors-and-ides)
  - [Workspace lifecycle](./docs/workspaces.md#workspace-lifecycle)
  - [Updating workspaces](./docs/workspaces.md#updating-workspaces)

## Comparison

Please file [an issue](https://github.com/coder/coder/issues/new) if any information is out of date. Also refer to: [What Coder is not](./docs/about.md#what-coder-is-not).

| Tool                                                        | Type     | Delivery Model     | Cost                          | Environments                                                                                                                                               |
| :---------------------------------------------------------- | :------- | :----------------- | :---------------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Coder](https://github.com/coder/coder)                     | Platform | OSS + Self-Managed | Pay your cloud                | All [Terraform](https://www.terraform.io/registry/providers) resources, all clouds, multi-architecture: Linux, Mac, Windows, containers, VMs, amd64, arm64 |
| [code-server](https://github.com/cdr/code-server)           | Web IDE  | OSS + Self-Managed | Pay your cloud                | Linux, Mac, Windows, containers, VMs, amd64, arm64                                                                                                         |
| [Coder (Classic)](https://coder.com/docs)                   | Platform | Self-Managed       | Pay your cloud + license fees | Kubernetes Linux Containers                                                                                                                                |
| [GitHub Codespaces](https://github.com/features/codespaces) | Platform | SaaS               | 2x Azure Compute              | Linux containers                                                                                                                                           |

---

_As of 5/27/22_

## Contributing

Read the [contributing docs](./docs/CONTRIBUTING.md).

Find our list of contributors [here](./docs/CONTRIBUTORS.md).
