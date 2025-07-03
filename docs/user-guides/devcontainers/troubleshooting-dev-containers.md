# Troubleshooting dev containers

If you encounter issues with dev containers in your workspace, review the steps here as well as the dev containers
[user](./index.md) and [admin](../../admin/templates/extending-templates/devcontainers.md#troubleshoot-common-issues) documentation.

## Container does not start

If your dev container fails to start:

1. Check the agent logs for error messages:

   - `/tmp/coder-agent.log`
   - `/tmp/coder-startup-script.log`
   - `/tmp/coder-script-[script_id].log`

1. Verify that Docker is running in your workspace.
1. Ensure the `devcontainer.json` file is valid.
1. Check that the repository has been cloned correctly.
1. Ensure the workspace image has Node/npm and the `devcontainers-cli` module installed.
1. Verify that the resource limits in your workspace are sufficient.

## Rebuild prompt does not appear

1. Confirm that you saved `devcontainer.json` in the correct repo path detected by Coder.
1. Check agent logs for `devcontainer build` errors.

## Known Limitations

Currently, dev containers are not compatible with the [prebuilt workspaces](../../admin/templates/extending-templates/prebuilt-workspaces.md).

If your template allows for prebuilt workspaces, do not select a prebuilt workspace if you plan to use a dev container.
