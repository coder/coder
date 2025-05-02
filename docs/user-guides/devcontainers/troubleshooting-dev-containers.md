# Troubleshooting dev containers

## Dev Container Not Starting

If your dev container fails to start:

1. Check the agent logs for error messages:

   - `/tmp/coder-agent.log`
   - `/tmp/coder-startup-script.log`
   - `/tmp/coder-script-[script_id].log`

1. Verify that Docker is running in your workspace.
1. Ensure the `devcontainer.json` file is valid.
1. Check that the repository has been cloned correctly.
1. Verify the resource limits in your workspace are sufficient.
