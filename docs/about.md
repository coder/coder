# About Coder

Coder is an open source platform for creating and managing developer workspaces on your preferred clouds and servers.

By building on top of common development inferfaces (SSH) and infrastructure tools (Terraform), Coder aims to make the process of **provisioning** and **accessing** remote workspaces approachable for organizations of various sizes and stages of cloud-native maturity.

> ⚠️ Coder v2 is in alpha and not ready for production use. You may be interested in [Coder v1](https://coder.com/docs) or [code-server](https://github.com/cdr/code-server).

## Why remote development

Migrating from local developer machines to remote servers is an increasingly common solution for developers[^1] and organizations[^2] alike. Remote development has a number of benefits:

- Speed: Server-grade compute speeds up operations in software development such as IDE loads, compiles, builds, and running large apps (monolyths or many microservices). 

- Environment management: Onboarding & troubleshooting development environments is automated using tools such as Terraform, nix, Docker, devcontainers, etc.

- Security: Source code and other data can be centralized on private servers or cloud, instead of local developer machines.

- Compatability: Remote workspaces share infrastructure configuration with other developer, staging, and production environments, reducing configuration drift.

- Accessibility: Devices such as light notebooks, Chromebooks, and iPads connect to remote workspaces via browser-based IDEs or remote IDE extensions.

## Why Coder?

The added layer of infrastructure control is a key differentiator from Coder v1 and other remote IDE platforms. This gives admins the ability to:

- support ARM, Windows, Linux, and MacOS workspaces
- modify pod/container spec: add disks, manage network policy, environment variables
- use VM/dedicated workspaces: develop with Kernel features, container knowledge not required
- enable persistant workspaces: just like a local machine, but faster and in the cloud

Coder includes [production-ready templates](./examples) for use on Kubernetes, AWS EC2, Google Cloud, Azure, and more.

## What Coder is not

- Coder is an infrastructure as code (IaC) platform. Terraform is the first IaC *provisioner* in Coder. As a result, Coder admins can define any Terraform resources can as Coder workspaces. 

- Coder is not a DevOps/CI platform. Coder workspaces can follow best practices for cloud workloads, but Coder is not responsible for how you define or deploy the software you write.

- Coder is not an online IDE. Instead, Coder has strong support for common editors such as VS Code, vim, and JetBrains, over HTTPS or SSH.

- Coder is not a collaboration platform. You can continue using git and IDE extensions for pull requests, code reviews, and pair programming.

- Coder is not SaaS/fully-managed. Install Coder on your cloud (AWS, GCP, Azure) or datacenter.

---

Next: [Templates](./templates.md)

[^1]: alexellis.io: [The Internet is my computer](https://blog.alexellis.io/the-internet-is-my-computer/)

[^2]: slack.engineering: [Development environments at Slack](https://slack.engineering/development-environments-at-slack)
