# Add an Envbuilder template

A Coder administrator adds an Envbuilder-compatible template to Coder. This
allows the template to prompt the developer for their dev container repository's
URL as a [parameter](../../../templates/extending-templates/parameters.md) when they create
their workspace. Envbuilder clones the repo and builds a container from the
`devcontainer.json` specified in the repo.

You can create template files through the Coder dashboard, CLI, or you can
choose a template from the
[Coder registry](https://registry.coder.com/templates):

<div class="tabs">

## Dashboard

1. In the Coder dashboard, select **Templates** then **Create Template**.
1. Use a
   [starter template](https://github.com/coder/coder/tree/main/examples/templates)
   or create a new template:

   - Starter template:

     1. Select **Choose a starter template**.
     1. Choose a template from the list or select **Devcontainer** from the
        sidebar to display only dev container-compatible templates.
     1. Select **Use template**, enter the details, then select **Create
        template**.

   - To create a new template, select **From scratch** and enter the templates
     details, then select **Create template**.

1. Edit the template files to fit your deployment.

## CLI

1. Use the `template init` command to initialize your choice of image:

   ```shell
   coder template init --id kubernetes-devcontainer
   ```

   A list of available templates is shown in the
   [templates_init](../../../../reference/cli/templates.md) reference.

1. `cd` into the directory and push the template to your Coder deployment:

   ```shell
   cd kubernetes-devcontainer && coder templates push
   ```

   You can also edit the files or make changes to the files before you push them
   to Coder.

## Registry

1. Go to the [Coder registry](https://registry.coder.com/templates) and select a
   dev container-compatible template.

1. Copy the files to your local device, then edit them to fit your needs.

1. Upload them to Coder through the CLI or dashboard:

   - CLI:

   ```shell
   coder templates push <template-name> -d <path to folder containing main.tf>
   ```

   - Dashboard:

   1. Create a `.zip` of the template files:

      - On Mac or Windows, highlight the files and then right click. A
        "compress" option is available through the right-click context menu.

      - To zip the files through the command line:

        ```shell
        zip templates.zip Dockerfile main.tf
        ```

   1. Select **Templates**.
   1. Select **Create Template**, then **Upload template**:

      ![Upload template](../../../../images/templates/upload-create-your-first-template.png)

   1. Drag the `.zip` file into the **Upload template** section and fill out the
      details, then select **Create template**.

      ![Upload the template files](../../../../images/templates/upload-create-template-form.png)

</div>

To set variables such as the namespace, go to the template in your Coder
dashboard and select **Settings** from the **⋮** (vertical ellipsis) menu:

<Image height="255px" src="../../../../images/templates/template-menu-settings.png" alt="Choose Settings from the template's menu" align="center" />

## Envbuilder Terraform provider

When using the
[Envbuilder Terraform provider](https://registry.terraform.io/providers/coder/envbuilder/latest/docs),
a previously built and cached image can be reused directly, allowing dev
containers to start instantaneously.

Developers can edit the `devcontainer.json` in their workspace to customize
their development environments:

```json
# …
{
  "features": {
      "ghcr.io/devcontainers/features/common-utils:2": {}
  }
}
# …
```

## Example templates

| Template                                                                                                            | Description                                                                                                                                                         |
|---------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Docker dev containers](https://github.com/coder/coder/tree/main/examples/templates/docker-devcontainer)            | Docker provisions a development container.                                                                                                                          |
| [Kubernetes dev containers](https://github.com/coder/coder/tree/main/examples/templates/kubernetes-devcontainer)    | Provisions a development container on the Kubernetes cluster.                                                                                                       |
| [Google Compute Engine dev container](https://github.com/coder/coder/tree/main/examples/templates/gcp-devcontainer) | Runs a development container inside a single GCP instance. It also mounts the Docker socket from the VM inside the container to enable Docker inside the workspace. |
| [AWS EC2 dev container](https://github.com/coder/coder/tree/main/examples/templates/aws-devcontainer)               | Runs a development container inside a single EC2 instance. It also mounts the Docker socket from the VM inside the container to enable Docker inside the workspace. |

Your template can prompt the user for a repo URL with
[parameters](../../../templates/extending-templates/parameters.md):

![Dev container parameter screen](../../../../images/templates/devcontainers.png)

## Dev container lifecycle scripts

Envbuilder supports the following lifecycle scripts: `onCreateCommand`,
`updateContentCommand`, `postCreateCommand`, and `postStartCommand`. These can
be used to fetch or update project dependencies before a user begins using the
workspace.

Lifecycle scripts are managed by project developers.

> [!NOTE]
> `onCreateCommand` runs only on the first start.
> `updateContentCommand` and `postCreateCommand` run each time the container
> is started. `postStartCommand` runs each time the container starts, but
> may be deferred to the init command depending on configuration.

### Unsupported lifecycle commands

Envbuilder does not support the following lifecycle commands:

- `initializeCommand`
- `postAttachCommand`
- `waitFor`

For a complete list of supported and unsupported dev container features, see the
[Envbuilder dev container spec support](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md)
documentation.

### Custom Dockerfile ENTRYPOINT

Envbuilder replaces the image `ENTRYPOINT` with its own binary during the build
process. Custom `ENTRYPOINT` instructions in your Dockerfile will not execute.
To run initialization commands, use lifecycle scripts such as
`postCreateCommand` or `postStartCommand` instead.

### SSH and Git credentials in lifecycle scripts

Envbuilder runs lifecycle scripts during the container build phase, before the
Coder agent starts. Because Coder-managed SSH keys and Git credentials are
injected by the agent, they are not available during lifecycle script execution.

This means operations that require SSH authentication, such as cloning private
git submodules, will fail if placed in `postCreateCommand` or other lifecycle
scripts.

To run commands that depend on Coder-managed SSH or Git credentials, use the
`coder_agent` startup script in your template instead:

```hcl
resource "coder_agent" "main" {
  # ...
  startup_script = <<-EOT
    set -e
    cd /workspaces/my-repo
    git submodule update --init --recursive
  EOT
}
```

The startup script runs after the agent starts and credentials are available.
It should be idempotent, as it runs each time the workspace starts.

## Next steps

- [Envbuilder security and caching](./envbuilder-security-caching.md)
