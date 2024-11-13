# Add a devcontainer template to Coder

A Coder administrator adds a devcontainer-compatible template to Coder
(Envbuilder).

When a developer creates their workspace, they enter their repository URL as a
[parameter](../../extending-templates/parameters.md). Envbuilder clones the repo
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
[parameters](../../extending-templates/parameters.md):

![Devcontainer parameter screen](../../../../images/templates/devcontainers.png)

## Devcontainer lifecycle scripts

The `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, and
`postStartCommand` lifecycle scripts are run each time the container is started.
This could be used, for example, to fetch or update project dependencies before
a user begins using the workspace.

Lifecycle scripts are managed by project developers.

## Next steps

- [Devcontainer security and caching](./devcontainer-security-caching.md)
