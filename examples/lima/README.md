---
name: Run Coder in Lima
description: Quickly stand up Coder using Lima
tags: [local, docker, vm, lima]
---

# Run Coder in Lima

This provides a sample [Lima](https://github.com/lima-vm/lima) configuration for Coder.
This lets you quickly test out Coder in a self-contained environment.

> Prerequisite: You must have `lima` installed and available to use this.

## Getting Started

- Run `limactl start --name=coder https://raw.githubusercontent.com/coder/coder/main/examples/lima/coder.yaml`
- You can use the configuration as-is, or edit it to your liking.

This will:

- Start an Ubuntu 22.04 VM
- Install Docker and Terraform from the official repos
- Install Coder using the [installation script](https://coder.com/docs/coder-oss/latest/install#installsh)
- Generates an initial user account `admin@coder.com` with a randomly generated password (stored in the VM under `/home/${USER}.linux/.config/coderv2/password`)
- Initializes a [sample Docker template](https://github.com/coder/coder/tree/main/examples/templates/docker-code-server) for creating workspaces

Once this completes, you can visit `http://localhost:3000` and start creating workspaces!

Alternatively, enter the VM with `limactl shell coder` and run `coder templates init` to start creating your own templates!

## Further Information

- To learn more about Lima, [visit the the project's GitHub page](https://github.com/lima-vm/lima/).
