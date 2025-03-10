# Generate and upload a Support Bundle to Coder Support

When you engage with Coder support to diagnose an issue with your deployment,
you may be asked to generate and upload a "Support Bundle" for offline analysis.
This document explains the contents of a support bundle and the steps to submit
a support bundle to Coder staff.

## What is a Support Bundle?

A support bundle is an archive containing a snapshot of information about your
Coder deployment.

It contains information about the workspace, the template it uses, running
agents in the workspace, and other detailed information useful for
troubleshooting.

It is primarily intended for troubleshooting connectivity issues to workspaces,
but can be useful for diagnosing other issues as well.

**While we attempt to redact sensitive information from support bundles, they
may contain information deemed sensitive by your organization and should be
treated as such.**

A brief overview of all files contained in the bundle is provided below:

> [!NOTE]
> Detailed descriptions of all the information available in the bundle is
> out of scope, as support bundles are primarily intended for internal use.

| Filename                          | Description                                                                                                |
|-----------------------------------|------------------------------------------------------------------------------------------------------------|
| `agent/agent.json`                | The agent used to connect to the workspace with environment variables stripped.                            |
| `agent/agent_magicsock.html`      | The contents of the HTTP debug endpoint of the agent's Tailscale Wireguard connection.                     |
| `agent/client_magicsock.html`     | The contents of the HTTP debug endpoint of the client's Tailscale Wireguard connection.                    |
| `agent/listening_ports.json`      | The listening ports detected by the selected agent running in the workspace.                               |
| `agent/logs.txt`                  | The logs of the selected agent running in the workspace.                                                   |
| `agent/manifest.json`             | The manifest of the selected agent with environment variables stripped.                                    |
| `agent/startup_logs.txt`          | Startup logs of the workspace agent.                                                                       |
| `agent/prometheus.txt`            | The contents of the agent's Prometheus endpoint.                                                           |
| `cli_logs.txt`                    | Logs from running the `coder support bundle` command.                                                      |
| `deployment/buildinfo.json`       | Coder version and build information.                                                                       |
| `deployment/config.json`          | Deployment [configuration](../reference/api/general.md#get-deployment-config), with secret values removed. |
| `deployment/experiments.json`     | Any [experiments](../reference/cli/server.md#--experiments) currently enabled for the deployment.          |
| `deployment/health.json`          | A snapshot of the [health status](../admin/monitoring/health-check.md) of the deployment.                  |
| `logs.txt`                        | Logs from the `codersdk.Client` used to generate the bundle.                                               |
| `network/connection_info.json`    | Information used by workspace agents used to connect to Coder (DERP map etc.)                              |
| `network/coordinator_debug.html`  | Peers currently connected to each Coder instance and the tunnels established between peers.                |
| `network/netcheck.json`           | Results of running `coder netcheck` locally.                                                               |
| `network/tailnet_debug.html`      | Tailnet coordinators, their heartbeat ages, connected peers, and tunnels.                                  |
| `workspace/build_logs.txt`        | Build logs of the selected workspace.                                                                      |
| `workspace/workspace.json`        | Details of the selected workspace.                                                                         |
| `workspace/parameters.json`       | Build parameters of the selected workspace.                                                                |
| `workspace/template.json`         | The template currently in use by the selected workspace.                                                   |
| `workspace/template_file.zip`     | The source code of the template currently in use by the selected workspace.                                |
| `workspace/template_version.json` | The template version currently in use by the selected workspace.                                           |

## How do I generate a Support Bundle?

1. Ensure your deployment is up and running. Generating a support bundle
   requires the Coder deployment to be available.

2. Ensure you have the Coder CLI installed on a local machine. See
   [installation](../install/index.md) for steps on how to do this.

   > [!NOTE]
   > It is recommended to generate a support bundle from a location
   > experiencing workspace connectivity issues.

3. Ensure you are [logged in](../reference/cli/login.md#login) to your Coder
   deployment as a user with the Owner privilege.

4. Run `coder support bundle [owner/workspace]`, and respond `yes` to the
   prompt. The support bundle will be generated in the current directory with
   the filename `coder-support-$TIMESTAMP.zip`.

   > While support bundles can be generated without a running workspace, it is
   > recommended to specify one to maximize troubleshooting information.

5. (Recommended) Extract the support bundle and review its contents, redacting
   any information you deem necessary.

6. Coder staff will provide you a link where you can upload the bundle along
   with any other necessary supporting files.

   > [!NOTE]
   > It is helpful to leave an informative message regarding the nature of
   > supporting files.

Coder support will then review the information you provided and respond to you
with next steps.
