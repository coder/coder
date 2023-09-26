---
display_name: Nomad
description: Provision Nomad Jobs as Coder workspaces
icon: ../../../site/static/icon/nomad.svg
maintainer_github: coder
verified: true
tags: [nomad, container]
---

# Remote Development on Nomad

Provision Nomad Jobs as [Coder workspaces](https://coder.com/docs/coder-v2/latest) with this example template.

<!-- TODO: Add screenshot -->

## Prerequisites

### Infrastructure

**Cluster**: This template requires an existing Kubernetes cluster

**Container Image**: This template uses the [codercom/enterprise-base:ubuntu image](https://github.com/coder/enterprise-images/tree/main/images/base) with some dev tools preinstalled. To add additional tools, extend this image or build it yourself.

### Authentication

This template authenticates using a `~/.kube/config`, if present on the server, or via built-in authentication if the Coder provisioner is running on Kubernetes with an authorized ServiceAccount. To use another [authentication method](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#authentication), edit the template.

## Architecture

This template provisions the following resources:

- Kubernetes pod (ephemeral)
- Kubernetes persistent volume claim (persistent on `/home/coder`)

This means, when the workspace restarts, any tools or files outside of the home directory are not persisted. To pre-bake tools into the workspace (e.g. `python3`), modify the container image. Alternatively, individual developers can [personalize](https://coder.com/docs/v2/latest/dotfiles) their workspaces with dotfiles.

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

# Develop in a Nomad Docker Container

This example shows how to use Nomad service tasks to be used as a development environment using docker and host csi volumes.

## Prerequisites

- [Nomad](https://www.nomadproject.io/downloads)
- [Docker](https://docs.docker.com/get-docker/)

## Setup

### 1. Start the CSI Host Volume Plugin

The CSI Host Volume plugin is used to mount host volumes into Nomad tasks. This is useful for development environments where you want to mount persistent volumes into your container workspace.

1. Login to the Nomad server using SSH.

2. Append the following stanza to your Nomad server configuration file and restart the nomad service.

   ```hcl
   plugin "docker" {
     config {
       allow_privileged = true
     }
   }
   ```

   ```shell
   sudo systemctl restart nomad
   ```

3. Create a file `hostpath.nomad` with following content:

   ```hcl
   job "hostpath-csi-plugin" {
     datacenters = ["dc1"]
     type = "system"

     group "csi" {
       task "plugin" {
         driver = "docker"

         config {
           image = "registry.k8s.io/sig-storage/hostpathplugin:v1.10.0"

           args = [
             "--drivername=csi-hostpath",
             "--v=5",
             "--endpoint=${CSI_ENDPOINT}",
             "--nodeid=node-${NOMAD_ALLOC_INDEX}",
           ]

           privileged = true
         }

         csi_plugin {
           id   = "hostpath"
           type = "monolith"
           mount_dir = "/csi"
         }

         resources {
           cpu    = 256
           memory = 128
         }
       }
     }
   }
   ```

4. Run the job:

   ```shell
   nomad job run hostpath.nomad
   ```

### 2. Setup the Nomad Template

1. Create the template by running the following command:

   ```shell
   coder template init nomad-docker
   cd nomad-docker
   coder template create
   ```

2. Set up Nomad server address and optional authentication:

3. Create a new workspace and start developing.
