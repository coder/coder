---
display_name: Docker (Devcontainer)
description: Provision envbuilder containers as Coder workspaces
icon: ../../../site/static/icon/docker.png
maintainer_github: coder
verified: true
tags: [container, docker, devcontainer]
---

# Remote Development on Docker Containers (with Devcontainers)

Provision Devcontainers as [Coder workspaces](https://coder.com/docs/workspaces) in Docker with this example template.

## Prerequisites

### Infrastructure

Coder must have access to a running Docker socket, and the `coder` user must be a member of the `docker` group:

```shell
# Add coder user to Docker group
sudo usermod -aG docker coder

# Restart Coder server
sudo systemctl restart coder

# Test Docker
sudo -u coder docker ps
```

## Architecture

Coder supports Devcontainers via [envbuilder](https://github.com/coder/envbuilder), an open source project. Read more about this in [Coder's documentation](https://coder.com/docs/templates/dev-containers).

This template provisions the following resources:

- Envbuilder cached image (conditional, persistent) using [`terraform-provider-envbuilder`](https://github.com/coder/terraform-provider-envbuilder)
- Docker image (persistent) using [`envbuilder`](https://github.com/coder/envbuilder)
- Docker container (ephemeral)
- Docker volume (persistent on `/workspaces`)

The Git repository is cloned inside the `/workspaces` volume if not present.
Any local changes to the Devcontainer files inside the volume will be applied when you restart the workspace.
Keep in mind that any tools or files outside of `/workspaces` or not added as part of the Devcontainer specification are not persisted.
Edit the `devcontainer.json` instead!

> **Note**
> This template is designed to be a starting point! Edit the Terraform to extend the template to support your use case.

## Docker-in-Docker

See the [Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/docker.md) for information on running Docker containers inside a devcontainer built by Envbuilder.

## Caching

To speed up your builds, you can use a container registry as a cache.
When creating the template, set the parameter `cache_repo` to a valid Docker repository.

For example, you can run a local registry:

```shell
docker run --detach \
  --volume registry-cache:/var/lib/registry \
  --publish 5000:5000 \
  --name registry-cache \
  --net=host \
  registry:2
```

Then, when creating the template, enter `localhost:5000/devcontainer-cache` for the parameter `cache_repo`.

See the [Envbuilder Terraform Provider Examples](https://github.com/coder/terraform-provider-envbuilder/blob/main/examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf/) for a more complete example of how the provider works.

> [!NOTE]
> We recommend using a registry cache with authentication enabled.
> To allow Envbuilder to authenticate with the registry cache, specify the variable `cache_repo_docker_config_path`
> with the path to a Docker config `.json` on disk containing valid credentials for the registry.
