---
name: Run Coder in Lima
description: Quickly stand up Coder using Lima
tags: [local, docker, incus, vm, lima]
---

# Run Coder in Lima

This provides sample [Lima](https://github.com/lima-vm/lima) configurations for Coder.
This lets you quickly test out Coder in a self-contained environment.
The Docker configuration runs workspaces in Docker containers; the Incus configuration runs workspaces in Incus system containers (with Docker available inside each workspace).

> Prerequisite: You must have `lima` installed and available to use this.

## Getting Started (Docker)

This configuration (`coder-docker.yaml`) creates a VM to run Coder workspaces in Docker.

- Run `limactl start --name=coder https://raw.githubusercontent.com/coder/coder/main/examples/lima/coder-docker.yaml`
- You can use the configuration as-is, or edit it to your liking.

This will:

- Start an Ubuntu 22.04 VM
- Install Docker and Terraform from the official repos
- Install Coder using the [installation script](../../docs/install/install.sh.md)
- Generate an initial user account `admin@coder.com` with a randomly generated password (stored in the VM under `/home/${USER}.linux/.config/coderv2/password`)
- Initialize a [sample Docker template](https://github.com/coder/coder/tree/main/examples/templates/docker) for creating workspaces

Once this completes, you can visit `http://localhost:3000` and start creating workspaces!

Alternatively, enter the VM with `limactl shell coder` and run `coder templates init` to start creating your own templates!

## Getting Started (Incus)

This configuration (`coder-incus.yaml`) creates a VM to run Coder workspaces in Incus.

- Run `limactl start --name=coder-incus https://raw.githubusercontent.com/coder/coder/main/examples/lima/coder-incus.yaml`
- You can use the configuration as-is, or edit it to your liking.

This will:

- Start a Debian 13 VM
- Install Incus from the Debian repos and Terraform via the Coder installer
- Install Coder using the [installation script](../../docs/install/install.sh.md)
- Generate an initial user account `admin@coder.com` with a randomly generated password (stored in the VM under `/home/${USER}.linux/.config/coderv2/password`)
- Initialize a [sample Incus template](https://github.com/coder/coder/tree/main/examples/templates/incus) for creating workspaces

Once this completes, you can visit `http://localhost:3000` and start creating workspaces!

Alternatively, enter the VM with `limactl shell coder-incus` and run `coder templates init` to start creating your own templates!

## Further Information

- To learn more about Lima, [visit the project's GitHub page](https://github.com/lima-vm/lima/).
