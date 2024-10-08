# Dev Containers

[Development containers](https://containers.dev) are an open source
specification for defining development environments.

[Envbuilder](https://github.com/coder/envbuilder) is an open source project by
Coder that runs dev containers via Coder templates and your underlying
infrastructure. It can run on Docker or Kubernetes.

There are several benefits to adding a devcontainer-compatible template to
Coder:

- Drop-in migration from Codespaces (or any existing repositories that use dev
  containers)
- Easier to start projects from Coder. Just create a new workspace then pick a
  starter devcontainer.
- Developer teams can "bring their own image." No need for platform teams to
  manage complex images, registries, and CI pipelines.

## How it works

A Coder admin adds a devcontainer-compatible template to Coder (envbuilder).
Then developers enter their repository URL as a
[parameter](../extending-templates/parameters.md) when they create their
workspace. [Envbuilder](https://github.com/coder/envbuilder) clones the repo and
builds a container from the `devcontainer.json` specified in the repo.

When using the [Envbuilder Terraform provider](#provider), a previously built
and cached image can be re-used directly, allowing instantaneous dev container
starts.

Developers can edit the `devcontainer.json` in their workspace to rebuild to
iterate on their development environments.

## Example templates

- [Devcontainers (Docker)](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-docker)
  provisions a development container using Docker.
- [Devcontainers (Kubernetes)](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-kubernetes)
  provisioners a development container on the Kubernetes.
- [Google Compute Engine (Devcontainer)](https://github.com/coder/coder/tree/main/examples/templates/gcp-devcontainer)
  runs a development container inside a single GCP instance. It also mounts the
  Docker socket from the VM inside the container to enable Docker inside the
  workspace.
- [AWS EC2 (Devcontainer)](https://github.com/coder/coder/tree/main/examples/templates/aws-devcontainer)
  runs a development container inside a single EC2 instance. It also mounts the
  Docker socket from the VM inside the container to enable Docker inside the
  workspace.

![Devcontainer parameter screen](../../../images/templates/devcontainers.png)

Your template can prompt the user for a repo URL with
[Parameters](../extending-templates/parameters.md).

## Authentication

You may need to authenticate to your container registry, such as Artifactory, or
git provider such as GitLab, to use Envbuilder. See the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/container-registry-auth.md)
for more information.

## Caching

To improve build times, dev containers can be cached. There are two main forms
of caching:

1. **Layer Caching** caches individual layers and pushes them to a remote
   registry. When building the image, Envbuilder will check the remote registry
   for pre-existing layers. These will be fetched and extracted to disk instead
   of building the layers from scratch.
2. **Image Caching** caches the _entire image_, skipping the build process
   completely (except for post-build lifecycle scripts).

Refer to the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/caching.md)
for more information.

## Envbuilder Terraform Provider

To support resuming from a cached image, use the
[Envbuilder Terraform Provider](https://github.com/coder/terraform-provider-envbuilder)
in your template. The provider will:

1. Clone the remote Git repository,
2. Perform a 'dry-run' build of the dev container in the same manner as
   Envbuilder would,
3. Check for the presence of a previously built image in the provided cache
   repository,
4. Output the image remote reference in SHA256 form, if found.

The above example templates will use the provider if a remote cache repository
is provided.

If you are building your own Devcontainer template, you can consult the
[provider documentation](https://registry.terraform.io/providers/coder/envbuilder/latest/docs/resources/cached_image).
You may also wish to consult a
[documented example usage of the `envbuilder_cached_image` resource](https://github.com/coder/terraform-provider-envbuilder/blob/main/examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf).

## Other features & known issues

Envbuilder provides two release channels:

- **Stable:** available at
  [`ghcr.io/coder/envbuilder`](https://github.com/coder/envbuilder/pkgs/container/envbuilder).
  Tags `>=1.0.0` are considered stable.
- **Preview:** available at
  [`ghcr.io/coder/envbuilder-preview`](https://github.com/coder/envbuilder/pkgs/container/envbuilder-preview).
  This is built from the tip of `main`, and should be considered
  **experimental** and prone to **breaking changes**.

Refer to the [Envbuilder GitHub repo](https://github.com/coder/envbuilder/) for
more information and to submit feature requests or bug reports.
