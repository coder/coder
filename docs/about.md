# About Coder

Coder is an open source platform for creating and managing developer workspaces
on your preferred clouds and servers.

By building on top of common development interfaces (SSH) and infrastructure tools (Terraform), Coder aims to make the process of **provisioning** and **accessing** remote workspaces approachable for organizations of various sizes and stages of cloud-native maturity.

> ⚠️ Coder v2 is in **alpha** state and is not ready for production use. For
> production environments, please consider [Coder v1](https://coder.com/docs) or
> [code-server](https://github.com/cdr/code-server).

## Why remote development

Migrating from local developer machines to workspaces hosted by cloud services
is an increasingly common solution for developers[^1] and organizations[^2]
alike. There are several benefits, including:

- **Increased speed:** Server-grade compute speeds up operations in software
  development, such as IDE loading, code compilation and building, and the
  running of large workloads (such as those for monolith or microservice
  applications)

- **Easier environment management:** Tools such as Terraform, nix, Docker,
  devcontainers, and so on make developer onboarding and the troubleshooting of
  development environments easier

- **Increase security:** Centralize source code and other data onto private
  servers or cloud services instead of local developer machines

- **Improved compatibility:** Remote workspaces share infrastructure
  configuration with other development, staging, and production environments,
  reducing configuration drift

- **Improved accessibility:** Devices such as lightweight notebooks,
  Chromebooks, and iPads can connect to remote workspaces via browser-based IDEs
  or remote IDE extensions

## Why Coder

The key difference between Coder v2 and other remote IDE platforms is the added
layer of infrastructure control. This additional layer allows admins to:

- Support ARM, Windows, Linux, and macOS workspaces
- Modify pod/container specs (e.g., adding disks, managing network policies,
  setting/updating environment variables)
- Use VM/dedicated workspaces, developing with Kernel features (no container
  knowledge required)
- Enable persistent workspaces, which are like local machines, but faster and
  hosted by a cloud service

Coder includes [production-ready templates](../examples/templates) for use with AWS EC2,
Azure, Google Cloud, Kubernetes, and more.

## What Coder is _not_

- Coder is not an infrastructure as code (IaC) platform. Terraform is the first
  IaC _provisioner_ in Coder, allowing Coder admins to define Terraform
  resources as Coder workspaces.

- Coder is not a DevOps/CI platform. Coder workspaces can follow best practices
  for cloud service-based workloads, but Coder is not responsible for how you
  define or deploy the software you write.

- Coder is not an online IDE. Instead, Coder supports common editors, such as VS
  Code, vim, and JetBrains, over HTTPS or SSH.

- Coder is not a collaboration platform. You can use git and dedicated IDE
  extensions for pull requests, code reviews, and pair programming.

- Coder is not a SaaS/fully-managed offering. You must host
  Coder on a cloud service (AWS, Azure, GCP) or your private data center.

Next: [Templates](./templates.md)

[^1]: alexellis.io: [The Internet is my computer](https://blog.alexellis.io/the-internet-is-my-computer/)
[^2]: slack.engineering: [Development environments at Slack](https://slack.engineering/development-environments-at-slack)
