# Troubleshooting dev containers

## Dev container not starting

If your dev container fails to start:

1. Check the agent logs for error messages:

   - `/tmp/coder-agent.log`
   - `/tmp/coder-startup-script.log`
   - `/tmp/coder-script-[script_id].log`

1. Verify Docker is available in your workspace (see below).
1. Ensure the `devcontainer.json` file is valid JSON.
1. Check that the repository has been cloned correctly.
1. Verify the resource limits in your workspace are sufficient.

## Docker not available

Dev containers require Docker, either via a running daemon (Docker-in-Docker) or
a mounted socket from the host. Your template determines which approach is used.

**If using Docker-in-Docker**, check that the daemon is running:

```console
sudo service docker status
sudo service docker start  # if not running
```

**If using a mounted socket**, verify the socket exists and is accessible:

```console
ls -la /var/run/docker.sock
docker ps  # test access
```

If you get permission errors, your user may need to be in the `docker` group.

## Finding your dev container agent

Use `coder show` to list all agents in your workspace, including dev container
sub-agents:

```console
coder show <workspace>
```

The agent name is derived from the workspace folder path. For details on how
names are generated, see [Agent naming](./index.md#agent-naming).

## SSH connection issues

If `coder ssh <workspace>.<agent>` fails:

1. Verify the agent name using `coder show <workspace>`.
1. Check that the dev container is running:

   ```console
   docker ps
   ```

1. Check the workspace agent logs for container-related errors:

   ```console
   grep -i container /tmp/coder-agent.log
   ```

## VS Code connection issues

VS Code connects to dev containers through the Coder extension. The extension
uses the sub-agent information to route connections through the parent workspace
agent to the dev container. If VS Code fails to connect:

1. Ensure you have the latest Coder VS Code extension.
1. Verify the dev container is running in the Coder dashboard.
1. Check the parent workspace agent is healthy.
1. Try restarting the dev container from the dashboard.

## Dev container features not working

If features from your `devcontainer.json` aren't being applied:

1. Rebuild the container to ensure features are installed fresh.
1. Check the container build output for feature installation errors.
1. Verify the feature reference format is correct:

   ```json
   {
     "features": {
       "ghcr.io/devcontainers/features/node:1": {}
     }
   }
   ```

## Slow container startup

If your dev container takes a long time to start:

1. **Use a pre-built image** instead of building from a Dockerfile. This avoids
   the image build step, though features and lifecycle scripts still run.
1. **Minimize features**. Each feature executes as a separate Docker layer
   during the image build, which is typically the slowest part. Changing
   `devcontainer.json` invalidates the layer cache, causing features to
   reinstall on rebuild.
1. **Check lifecycle scripts**. Commands in `postStartCommand` run on every
   container start. Commands in `postCreateCommand` run once per build, so
   they execute again after each rebuild.

## Git submodules or private repos failing in lifecycle scripts

If `git submodule update --init` or other SSH-dependent commands fail when
placed in `postCreateCommand` or `postStartCommand`:

- **Cause**: Dev container lifecycle scripts run before the Coder agent starts.
  Coder-managed SSH keys and Git credentials are not yet available during
  lifecycle script execution.
- **Fix**: Move SSH-dependent commands to the `coder_agent` startup script in
  your template. The startup script runs after the agent initializes and
  credentials become available. Ask your template administrator to add the
  commands to the agent's `startup_script` block.

For Envbuilder-based templates, see
[SSH and Git credentials in lifecycle scripts](../../admin/integrations/devcontainers/envbuilder/add-envbuilder.md#ssh-and-git-credentials-in-lifecycle-scripts)
for details and a template example.

## Custom ENTRYPOINT not executing (Envbuilder)

If your custom Dockerfile `ENTRYPOINT` does not run in an Envbuilder-based
workspace:

- **Cause**: Envbuilder replaces the image entrypoint with its own binary
  during the build process.
- **Fix**: Use dev container lifecycle scripts such as `postCreateCommand` or
  `postStartCommand` for initialization logic instead. For commands that need
  Coder-managed credentials, use the agent startup script as described above.

## Getting more help

If you continue to experience issues:

1. Collect logs from `/tmp/coder-agent.log` (both workspace and container).
1. Note the exact error messages.
1. Check [Coder GitHub issues](https://github.com/coder/coder/issues) for
   similar problems.
1. Contact your Coder administrator for template-specific issues.
