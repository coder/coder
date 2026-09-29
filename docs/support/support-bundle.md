---
title: Generate a support bundle
---

If you use a Coder workspace, you can collect a support bundle from the CLI or your IDE to share with Coder support.
A support bundle is a ZIP archive of deployment, workspace, agent, and connection diagnostics.

> [!WARNING]
> Review the archive before sharing it through a trusted channel.
> Redaction cannot guarantee that logs, settings, template source, or workspace files are free of credentials or other sensitive data.

<a id="what-is-a-support-bundle"></a>

## Bundle contents

The CLI collects deployment and connection diagnostics, plus workspace and agent details when you specify a workspace.
Agent logs include the active log and retained rotated logs modified in the last 24&nbsp;hours, capped at 100&nbsp;MiB.
The lookback doesn't guarantee a full 24&nbsp;hours of history because rotation can remove older logs.
Additional workspace files are opt-in through `--workspace-file`.
IDE integrations add their own diagnostics, which aren't included by the CLI alone.

Any authenticated user can generate a bundle.
Your permissions, deployment configuration, and agent connectivity determine which data is available.
Users with the Owner role get the most complete bundle; unavailable data can leave files empty or JSON values `null`.

Choose a collection method:

- [CLI](#generate-with-the-cli)
- [VS Code](#vs-code)
- [JetBrains Toolbox](#jetbrains-toolbox)

<details>
<summary>Detailed archive contents</summary>

The archive contains the following files when the corresponding data is available:

| Filename                                      | Description                                                                                                                                                                            |
|-----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `agent/agent.json`                            | The agent used to connect to the workspace with environment variables stripped.                                                                                                        |
| `agent/agent_magicsock.html`                  | The contents of the HTTP debug endpoint of the agent's Tailscale Wireguard connection.                                                                                                 |
| `agent/client_magicsock.html`                 | The contents of the HTTP debug endpoint of the client's Tailscale Wireguard connection.                                                                                                |
| `agent/listening_ports.json`                  | The listening ports detected by the selected agent running in the workspace.                                                                                                           |
| `agent/logs.txt`                              | Active agent log plus rotated agent logs modified in the last 24&nbsp;hours, capped at 100&nbsp;MiB.                                                                                   |
| `agent/workspace_files/collection_errors.txt` | Workspace file entries dropped while assembling the bundle, such as entries exceeding the size budget. Only present when entries were dropped.                                         |
| `agent/workspace_files/files/`                | Files collected from inside the remote workspace with `--workspace-file`. Only present when workspace paths are requested.                                                             |
| `agent/workspace_files/manifest.json`         | Describes the remote workspace file collection: requested patterns, collected files, per-path errors, truncation, and applied limits. Only present when workspace paths are requested. |
| `agent/manifest.json`                         | The manifest of the selected agent with environment variables stripped.                                                                                                                |
| `agent/startup_logs.txt`                      | Startup logs of the workspace agent.                                                                                                                                                   |
| `agent/peer_diagnostics.json`                 | Connection details for the selected agent's peer.                                                                                                                                      |
| `agent/ping_result.json`                      | Results of a connection check to the selected agent.                                                                                                                                   |
| `agent/prometheus.txt`                        | The contents of the agent's Prometheus endpoint.                                                                                                                                       |
| `cli_logs.txt`                                | Logs from running the `coder support bundle` command.                                                                                                                                  |
| `deployment/buildinfo.json`                   | Coder version and build information.                                                                                                                                                   |
| `deployment/config.json`                      | Deployment [configuration](../reference/api/general.md#get-deployment-config), with secret values removed. *Requires Owner role.*                                                      |
| `deployment/experiments.json`                 | Any [experiments](../reference/cli/server/index.md#--experiments) currently enabled for the deployment.                                                                                |
| `deployment/health.json`                      | A snapshot of the [health status](../admin/monitoring/health-check.md) of the deployment. *Requires Owner role.*                                                                       |
| `deployment/stats.json`                       | Aggregated workspace and session metrics, subject to your permissions.                                                                                                                 |
| `deployment/entitlements.json`                | Feature entitlements, when available.                                                                                                                                                  |
| `deployment/health_settings.json`             | Dismissed health checks, subject to your permissions.                                                                                                                                  |
| `deployment/workspaces.json`                  | Workspaces you can access, with agent environment values redacted. Limited to 10 workspaces by default.                                                                                |
| `deployment/prometheus.txt`                   | Control plane Prometheus metrics, when enabled and accessible.                                                                                                                         |
| `license-status.txt`                          | License status, when available.                                                                                                                                                        |
| `logs.txt`                                    | Logs from the `codersdk.Client` used to generate the bundle.                                                                                                                           |
| `network/connection_info.json`                | Information used by workspace agents used to connect to Coder (DERP map etc.)                                                                                                          |
| `network/coordinator_debug.html`              | Peers currently connected to each Coder instance and the tunnels established between peers. *Requires Owner role.*                                                                     |
| `network/interfaces.json`                     | Network interfaces on the machine running the CLI.                                                                                                                                     |
| `network/netcheck.json`                       | Results of running `coder netcheck` locally.                                                                                                                                           |
| `network/tailnet_debug.html`                  | Tailnet coordinators, their heartbeat ages, connected peers, and tunnels. *Requires Owner role.*                                                                                       |
| `workspace/build_logs.txt`                    | Build logs of the selected workspace.                                                                                                                                                  |
| `workspace/workspace.json`                    | Details of the selected workspace.                                                                                                                                                     |
| `workspace/parameters.json`                   | Build parameters of the selected workspace.                                                                                                                                            |
| `workspace/template.json`                     | The template currently in use by the selected workspace.                                                                                                                               |
| `workspace/template_file.zip`                 | The source code of the template currently in use by the selected workspace.                                                                                                            |
| `workspace/template_version.json`             | The template version currently in use by the selected workspace.                                                                                                                       |
| `templates/<name>/`                           | Template details, active version, and source archive requested with `--template`.                                                                                                      |
| `pprof/`                                      | Control plane profiling data requested with `--pprof`; agent profiles are under `pprof/agent/`.                                                                                        |
| `vscode-logs/`                                | Only present when generated from the VS Code Coder Remote extension. Includes logs, redacted settings, and local telemetry files.                                                      |

</details>

<a id="how-do-i-generate-a-support-bundle"></a>

## Generate with the CLI

Use a machine on the network where you connect to your workspace so that the bundle captures that machine's connection diagnostics.
Your Coder deployment must be available.
A running, reachable workspace provides the most complete agent diagnostics.

1. [Install the Coder CLI](../install/index.md) on your local machine.
1. [Log in](../reference/cli/login/index.md) to your deployment.
1. Run the following command, replacing `owner/workspace` with your workspace's owner and name:

   ```sh
   coder support bundle owner/workspace
   ```

1. Review the collection notice and enter `yes` to confirm.

The CLI saves `coder-support-<timestamp>.zip` in the current directory and prints `Wrote support bundle to` followed by its path.
To select an agent in a workspace with multiple agents, append its name: `coder support bundle owner/workspace agent-name`.
If you omit the workspace when running inside one, the CLI infers the workspace and agent from the environment.
Outside a workspace, omitting the workspace produces deployment and local network diagnostics without workspace-specific data.

### Include workspace files

To include editor or service logs from the workspace, add one `--workspace-file` flag per path or glob.
These files come from the remote workspace, not from the machine running the CLI.
The workspace agent must be reachable and support workspace file collection.

> [!WARNING]
> Requested workspace files are not redacted and can contain tokens, credentials, or source code.
> Review `agent/workspace_files/` before sharing the archive to avoid exposing sensitive data.

Run the following command with the paths you want to collect.
Quote each pattern so that your local shell doesn't expand it:

```sh
coder support bundle owner/workspace \
  --workspace-file '$HOME/.vscode-server/data/logs/**/*.log' \
  --workspace-file '$HOME/.local/share/code-server/coder-logs/**/*.log'
```

After confirmation, the CLI writes a ZIP archive with collected files under `agent/workspace_files/files/`.
The `agent/workspace_files/manifest.json` file records requested patterns, collected files, errors, truncation, and limits.
An agent that doesn't support collection records that limitation in the manifest instead of collecting the requested files.

The workspace agent evaluates paths and globs:

- Environment variables such as `$HOME` expand inside the workspace.
- Paths must resolve to absolute paths or start with `~/`, which resolves to the agent user's home directory.
- Direct paths follow symlinks; glob traversal doesn't follow symlinks.
- Collection is limited to 10,000 files and 100&nbsp;MiB in total.
- Each file contributes at most its last 10&nbsp;MiB, or less if the total budget is nearly exhausted, with truncation recorded in the manifest.

### Customize collection

Use these options to adjust the bundle:

| Option                           | Effect                                                                                                                                                                                                           |
|----------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--output-file <path>`           | Choose where to save the ZIP archive.                                                                                                                                                                            |
| `--workspaces-total-cap <count>` | Set the maximum number of entries in `deployment/workspaces.json`. Defaults to 10; zero or a negative value removes the cap.                                                                                     |
| `--template <name>`              | Include a template's active version and source, independently of the selected workspace. Use `org_name/template_name` if the name exists in multiple organizations.                                              |
| `--pprof`                        | Include control plane and agent profiling data. Requires Coder version 2.28.0 or later. CPU and trace profiles sample 30&nbsp;seconds per target; collecting both control plane and agent profiles takes longer. |

For all options, refer to the [`coder support bundle` reference](../reference/cli/support/bundle.md).

## Generate from your IDE

Use your IDE's collection action to include its local diagnostics alongside the Coder bundle.
Running the CLI separately doesn't collect those local IDE files.

### VS Code

Use the [Coder Remote extension](../user-guides/workspace-access/vscode.md) version 1.16.0 or later for this workflow.
Sign in to your Coder deployment in the extension before collecting a bundle.
Bundle generation requires Coder CLI version 2.10.0 or later.

> [!WARNING]
> Remote workspace logs are not redacted and can contain credentials or source code.
> Review these files before sharing the archive to avoid exposing sensitive data.

1. Open the Command Palette.
1. Run **Coder: Create Support Bundle**.
1. If prompted, select a running workspace.
1. Review **Create a support bundle?** and select **Continue**.
1. Choose a local destination in **Save Support Bundle**.
   The extension confirms the saved archive's location and offers **Reveal in File Explorer**.
1. [Review the archive before sharing it](#review-and-share-the-bundle).

The extension runs `coder support bundle` and adds local diagnostics under `vscode-logs/`:

- Coder extension logs from recent windows and sessions.
- SSH proxy and Remote-SSH logs.
- Selected VS Code settings, with sensitive settings masked.
- Local telemetry files, when available.

With extension version 1.16.4 or later, collected logs can include session identifiers, workspace and agent state changes, and recent buffered connection details.

With Coder CLI version 2.36.0 or later, the extension also requests remote editor server logs through workspace file collection.
Collecting these files requires a reachable workspace agent that supports workspace file collection.
These files appear under `agent/workspace_files/`, separately from the local VS Code diagnostics.

For telemetry controls and retention, refer to [VS Code local telemetry](../user-guides/workspace-access/vscode.md#local-telemetry).
For extension installation and settings, refer to the [Coder Remote README](https://github.com/coder/vscode-coder/blob/main/README.md#getting-started).

### JetBrains Toolbox

Use the [Coder plugin in JetBrains Toolbox](../user-guides/workspace-access/jetbrains/toolbox.md) to collect diagnostics for a specific workspace.
The plugin build that includes Coder support bundles requires JetBrains Toolbox version 3.7.2 or later.
The plugin uses its existing CLI login to generate the Coder bundle.
Bundle generation requires Coder CLI version 2.10.0 or later and access to the deployment.

1. Open the Coder plugin's **Workspaces** page in JetBrains Toolbox.
1. Open the action menu for the workspace you want to collect.
1. Select **Collect logs**.
   Toolbox collects a diagnostic ZIP with `coder-support.zip` in the selected environment's diagnostic directory, alongside Toolbox diagnostics.
1. Open the nested `coder-support.zip` in the collected Toolbox archive.
1. [Review both archives before sharing them](#review-and-share-the-bundle).

The action is also available from the individual workspace view.

The general Toolbox **Collect logs and diagnostic data** action under **Settings** > **About** doesn't generate a Coder support bundle.
Use the workspace-specific **Collect logs** action instead.

If Coder bundle generation fails, Toolbox preserves its other diagnostics and attempts to include `coder-support-error.txt` instead of a partial Coder archive.

For plugin-specific collection details, refer to [Coder support bundles in the Toolbox plugin README](https://github.com/coder/coder-jetbrains-toolbox/blob/main/README.md#coder-support-bundles).

## Review and share the bundle

1. Extract the ZIP archive.
1. Review its contents and remove sensitive information before sharing.
1. Create a new ZIP archive from the reviewed contents.
1. Upload the reviewed archive through the link Coder support provides.

Include a description of the issue and any supporting files requested by Coder support.
