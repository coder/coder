# Development containers on Coder

A development container (dev container or devcontainer) is an
[open source specification](https://containers.dev/implementors/spec/) for
defining containerized development environments.

Leverage Coder with dev containers and apply cloud-native security practices to
traditional ticket-ops and approval-ops workflows to help enable developers to
self-service.

## Benefits of devcontainers

There are several benefits to adding a devcontainer-compatible template to
Coder:

- Reliability and scalability
- Improved security
- Performance efficiency
- Cost Optimization

### Reliability and scalability

Envbuilder is an open source project independently packaged and versioned from
the centralized Coder open source project. This means that it can be used with
Coder, but it is not required. It also means that Dev Container builds can scale
independently of the Coder control plane and even run in CI/CD.

### Improved security

Since Coder and Envbuilder run on your own infrastructure, you can use firewalls
and cluster-level policies to ensure Envbuilder only downloads packages from
your secure registry powered by JFrog Artifactory or Sonatype Nexus.
Additionally, Envbuilder can be configured to push the full image back to your
registry for additional security scanning.

This means that Coder admins can still require hardened base images and
packages, while still allowing developer self service.

### Performance efficiency

Create a unique image for each project to reduce the dependency size of any
given project.

Envbuilder has various caching modes to ensure workspaces start as fast as
possible, such as layer caching and even full image caching and fetching via the
[Envbuilder Terraform provider](https://registry.terraform.io/providers/coder/envbuilder/latest/docs).

### Cost optimization

By creating unique images per-project, you remove unnecessary dependencies and
reduce the workspace size and resource consumption of any given project. Full
image caching ensures optimal start and stop times.

## When to use a dev container

A development container

## Coder Envbuilder

Envbuilder is an open source project by Coder that runs dev containers via Coder
templates and your underlying infrastructure. It can run on Docker or
Kubernetes.

Envbuilder uses the Dev Container standard used in VS Code Local, Daytona,
DevPod, and Codespaces. This format is already familiar to developers and can
simplify migration. This allows developers to take control of their own
environments, while still following cloud-native security best practices. See
the [Security section](#devcontainer-security) for more information.

## Devcontainer Features

[Devcontainer Features](https://containers.dev/implementors/features/) allow
owners of a project to specify self-contained units of code and runtime
configuration that can be composed together on top of an existing base image.
This is a good place to install project-specific tools, such as
language-specific runtimes and compilers.

## Add a devcontainer template to Coder

A Coder admin adds a devcontainer-compatible template to Coder (Envbuilder).

When a developer creates their workspace, they enter their repository URL as a
[parameter](../extending-templates/parameters.md). Envbuilder clones the repo
and builds a container from the `devcontainer.json` specified in the repo.

Admin:

1. Use a [devcontainer template](https://registry.coder.com/templates)
1. Create a template with the template files from the registry (git clone,
   upload files, or copy paste)
1. In template settings > variables > set necessary variables such as the
   namespace
1. Create a workspace from the template
1. Choose a **Repository** URL
   - The repo must have a `.devcontainer` directory with `devcontainer.json`

When using the
[Envbuilder Terraform provider](https://github.com/coder/terraform-provider-envbuilder),
a previously built and cached image can be reused directly, allowing dev
containers to start instantaneously.

Developers can edit the `devcontainer.json` in their workspace to customize
their development environments:

```json
…
"customizations": {
    // Configure properties specific to VS Code.
        "vscode": {
            "settings": {
                "editor.tabSize": 4,
                "editor.detectIndentation": false
                "editor.insertSpaces": true
                "files.trimTrailingWhitespace": true
            },
  "extensions": [
                "github.vscode-pull-request-github",
  ]
        }
},
…
```

## Example templates

- [Docker devcontainers](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-docker)
  - Docker provisions a development container.
- [Kubernetes devcontainers](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-kubernetes)
  - Provisions a development container on the Kubernetes.
- [Google Compute Engine devcontainer](https://github.com/coder/coder/tree/main/examples/templates/gcp-devcontainer)
  - Runs a development container inside a single GCP instance. It also mounts
    the Docker socket from the VM inside the container to enable Docker inside
    the workspace.
- [AWS EC2 devcontainer](https://github.com/coder/coder/tree/main/examples/templates/aws-devcontainer)
  - Runs a development container inside a single EC2 instance. It also mounts
    the Docker socket from the VM inside the container to enable Docker inside
    the workspace.

Your template can prompt the user for a repo URL with
[parameters](../extending-templates/parameters.md):

![Devcontainer parameter screen](../../../images/templates/devcontainers.png)

## Devcontainer security

Ensure Envbuilder can only pull images and artifacts by configuring it with your
existing HTTP proxies, firewalls, and artifact managers.

### Configure registry authentication

You may need to authenticate to your container registry, such as Artifactory, or
Git provider such as GitLab, to use Envbuilder. See the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/container-registry-auth.md)
for more information.

## Authentication

You may need to authenticate to your container registry, such as Artifactory, or
git provider such as GitLab, to use Envbuilder. See the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/container-registry-auth.md)
for more information.

## Layer and image caching

To improve build times, dev containers can be cached. There are two main forms
of caching:

- **Layer caching**

  - Caches individual layers and pushes them to a remote registry. When building
    the image, Envbuilder will check the remote registry for pre-existing layers
    These will be fetched and extracted to disk instead of building the layers
    from scratch.

- **Image caching**
  - Caches the entire image, skipping the build process completely (except for
    post-build [lifecycle scripts](#devcontainer-lifecycle-scripts)).

Refer to the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/caching.md)
for more information.

Note that caching requires push access to a registry, and may require approval.

### Image caching

To support resuming from a cached image, use the
[Envbuilder Terraform Provider](https://github.com/coder/terraform-provider-envbuilder)
in your template. The provider will:

1. Clone the remote Git repository,
1. Perform a 'dry-run' build of the dev container in the same manner as
   Envbuilder would,
1. Check for the presence of a previously built image in the provided cache
   repository,
1. Output the image remote reference in SHA256 form, if it finds one.

The example templates listed above will use the provider if a remote cache
repository is provided.

If you are building your own Devcontainer template, you can consult the
[provider documentation](https://registry.terraform.io/providers/coder/envbuilder/latest/docs/resources/cached_image).
You may also wish to consult a
[documented example usage of the `envbuilder_cached_image` resource](https://github.com/coder/terraform-provider-envbuilder/blob/main/examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf).

## Devcontainer lifecycle scripts

The `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, and
`postStartCommand` lifecycle scripts are run each time the container is started.
This could be used, for example, to fetch or update project dependencies before
a user begins using the workspace.

Lifecycle scripts are managed by project developers.

## Release channels

Envbuilder provides two release channels:

- **Stable**
  - Available at
    [`ghcr.io/coder/envbuilder`](https://github.com/coder/envbuilder/pkgs/container/envbuilder).
    Tags `>=1.0.0` are considered stable.
- **Preview**
  - Available at
    [`ghcr.io/coder/envbuilder-preview`](https://github.com/coder/envbuilder/pkgs/container/envbuilder-preview).
    Built from the tip of `main`, and should be considered experimental and
    prone to breaking changes.

Refer to the
[Envbuilder GitHub repository](https://github.com/coder/envbuilder/) for more
information and to submit feature requests or bug reports.

## Known issues

- Image caching: error pushing image

  - `BLOB_UNKNOWN: Manifest references unknown blob(s)`
  - [Issue 385](https://github.com/coder/envbuilder/issues/385)

- Support for VS Code Extensions requires a workaround.

  - [Issue 68](https://github.com/coder/envbuilder/issues/68#issuecomment-1805974271)

- Envbuilder does not support Volume Mounts

- Support for lifecycle hooks is limited.
  ([Issue](https://github.com/coder/envbuilder/issues/395))
  - Supported:
    - `onCreateCommand`
    - `updateContentCommand`
    - `postCreateCommand`
    - `postStartCommand`
  - Not supported:
    - `initializeCommand`
    - `postAttachCommand`
    - `waitFor`

Visit the
[Envbuilder repository](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md)
for a full list of supported features and known issues.
