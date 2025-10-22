---
display_name: Incus System Container with Docker
description: Develop in an Incus System Container with Docker using incus
icon: ../../../site/static/icon/lxc.svg
maintainer_github: coder
verified: true
tags: [local, incus, lxc, lxd]
---

# Incus System Container with Docker

Develop in an Incus System Container and run nested Docker containers using Incus on your local infrastructure.

## Prerequisites

1. Install [Incus](https://linuxcontainers.org/incus/) on the same machine as Coder.
2. Allow Coder to access the Incus socket.

   - If you're running Coder as system service, run `sudo usermod -aG incus-admin coder` and restart the Coder service.
   - If you're running Coder as a Docker Compose service, get the group ID of the `incus-admin` group by running `getent group incus-admin` and add the following to your `compose.yaml` file:

     ```yaml
     services:
       coder:
         volumes:
           - /var/lib/incus/unix.socket:/var/lib/incus/unix.socket
         group_add:
           - 996 # Replace with the group ID of the `incus-admin` group
     ```

3. Create a storage pool named `coder` and `btrfs` as the driver by running `incus storage create coder btrfs`.

## Usage

> **Note:** this template requires using a container image with cloud-init installed such as `ubuntu/jammy/cloud/amd64`.

1. Run `coder templates init -id incus`
1. Select this template
1. Follow the on-screen instructions

## Extending this template

See the [lxc/incus](https://registry.terraform.io/providers/lxc/incus/latest/docs) Terraform provider documentation to
add the following features to your Coder template:

- HTTPS incus host
- Volume mounts
- Custom networks
- More

We also welcome contributions!
