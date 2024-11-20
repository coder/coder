# Add a dev container template to Coder

A Coder administrator adds a dev container-compatible template to Coder
(Envbuilder).

When a developer creates their workspace, they enter their repository URL as a
[parameter](../../extending-templates/parameters.md). Envbuilder clones the repo
and builds a container from the `devcontainer.json` specified in the repo.

//tabs UI | CLI | Registry (/starter-templates?tag=devcontainer)

<div class="tabs">

## Dashboard

1. In the Coder dashboard, select **Templates** then **Create Template**.
1. To use a [starter template](https://github.com/coder/coder/tree/main/examples/templates), select **Choose a starter template**.
  - Select **From scratch** to create a new template and enter information and file contents manually.

## CLI

1. Use the `template init` command to initialize your choice of image:

   ```shell
   coder template init --id devcontainer-kubernetes
   ```

   A list of available templates is shown in the [templates_init](../../../../reference/cli/templates_init#--id) reference.

1. `cd` into the directory and push the template to your Coder deployment:

   ```shell
   cd $_ && coder templates push
   ```

   You can also edit the files or make changes to the files before you push them to Coder.

## Registry

1. Go to the [Coder Registry](https://registry.coder.com/templates?tag=devcontainer) and select a dev container-compatible template.

1. Copy the files to your local device, then edit them to fit your needs.

1. Upload them to Coder through the CLI or dashboard:

   - CLI:
  
     ```shell
     coder template push
     ```

   - Dashboard:

     1. Create a `.zip` of the template files:

         - On Mac or Windows, highlight the files and then right click. A "compress" option is available through the right-click context menu.

         - To zip the files through the command line:

           ```shell
           zip templates.zip Dockerfile main.tf
           ```

     1. Select **Templates**.
     1. Select **Create Template**, then **Upload template**:

       ![Upload template](../../../../images/templates/upload-create-your-first-template.png)
       
     1. Drag the `.zip` file into the **Upload template** section and fill out the details, then select **Create template**.

       ![Upload the template files](../../../../images/templates/upload-create-template-form.png)
    
     1. Once the upload completes, select **Templates** from the top to deploy it to a new workspace.

</div>

Admin:

1. Use a [dev container template](https://registry.coder.com/templates)
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

- [Docker dev containers](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-docker)
  - Docker provisions a development container.
- [Kubernetes dev ontainers](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-kubernetes)
  - Provisions a development container on the Kubernetes.
- [Google Compute Engine dev container](https://github.com/coder/coder/tree/main/examples/templates/gcp-devcontainer)
  - Runs a development container inside a single GCP instance. It also mounts
    the Docker socket from the VM inside the container to enable Docker inside
    the workspace.
- [AWS EC2 dev container](https://github.com/coder/coder/tree/main/examples/templates/aws-devcontainer)
  - Runs a development container inside a single EC2 instance. It also mounts
    the Docker socket from the VM inside the container to enable Docker inside
    the workspace.

Your template can prompt the user for a repo URL with
[parameters](../../extending-templates/parameters.md):

![Dev container parameter screen](../../../../images/templates/devcontainers.png)

## Dev container lifecycle scripts

The `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, and
`postStartCommand` lifecycle scripts are run each time the container is started.
This could be used, for example, to fetch or update project dependencies before
a user begins using the workspace.

Lifecycle scripts are managed by project developers.

## Next steps

- [Dev container security and caching](./devcontainer-security-caching.md)
