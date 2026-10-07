---
title: Connection logs
---

> [!NOTE]
> Connection logs require a
> [Premium license](https://coder.com/pricing#compare-plans).
> For more details, [contact your account team](https://coder.com/contact).

The **Connection Log** page in the dashboard allows Auditors to monitor workspace agent connections.

## Workspace App Connections

The connection log contains a complete record of all workspace app connections.
These originate from within the Coder deployment, and thus the connection log
is a source of truth for these events.

## Browser Port Forwarding

The connection log contains a complete record of all workspace port forwarding
performed via the dashboard.

## SSH and IDE Sessions

The connection log aims to capture a record of all workspace SSH and IDE sessions.
These events are reported by workspace agents, and their receipt by the control
plane is not guaranteed.

Agent-reported events do not identify the Coder user who connected. To
attribute SSH and IDE activity to a user, correlate them with tunnel
events for the same workspace and agent.

## Tunnel Connections

The connection log records the authorization decision for each request to add a tunnel to a workspace agent.
Accepted requests have status code `101`, and denied requests have status code `403`.
Tunnel events include the authenticated user's identity, IP address, and user agent.

Keep the following in mind when interpreting tunnel events:

- A tunnel event records an authorization decision, not how the tunnel was used.
  Clients such as `coder ssh`, `coder port-forward`, `coder ping`, `coder speedtest`, Coder Desktop, and IDE extensions can request tunnels.
- Tunnel events are deduplicated per workspace agent, actor, IP address, client, and authorization result.
  Clients automatically re-request tunnels after network interruptions or server restarts.
  These requests do not produce new events while a session is active.
  A new event is recorded after one hour of inactivity, or when the actor, IP address, client, or result changes.
- Like workspace app connections, tunnel events are point-in-time records.
  They have no close time and are excluded from `status:` filter results.

## How to Filter Connection Logs

You can filter connection logs by the following parameters:

- `organization` - The name or ID of the organization of the workspace being
     connected to.
- `workspace_owner` - The username of the owner of the workspace being connected
    to.
- `type` - The deprecated connection type, kept for compatibility.
  Use `method` and `app` instead.
  It accepts `ssh`, `vscode`, `jetbrains`, `reconnecting_pty`, `workspace_app`, `port_forwarding`, or `tunnel`.
  `type:vscode` and `type:jetbrains` match SSH connections from apps in the VS Code or JetBrains IDE family.
  `type:ssh` matches every other SSH connection, including connections from unrecognized apps and connections with no reported app.
- `method` - How the connection was established: `ssh`, `reconnecting_pty`, `workspace_app`, `port_forwarding`, or `tunnel`.
  `method:ssh` matches every SSH connection, whichever app made it.
- `app` - The client app that made an SSH or reconnecting PTY connection, such as `vscode` or `jetbrains`.
  Coder lowercases the value and replaces hyphens with underscores before it matches.
  Workspace app slugs and forwarded ports don't match this filter.
- `username`: The name of the user who initiated the connection.
   Results do not include agent-reported SSH or IDE sessions.
- `user_email`: The email of the user who initiated the connection.
   Results do not include agent-reported SSH or IDE sessions.
- `connected_after`: The time after which the connection started.
   Uses the RFC3339Nano format.
- `connected_before`: The time before which the connection started.
   Uses the RFC3339Nano format.
- `workspace_id`: The ID of the workspace being connected to.
- `connection_id`: The ID of the connection.
- `status`: The status of the connection, either `ongoing` or `completed`.
  Only SSH and reconnecting PTY connections have a status.
  A connection is `completed` after Coder receives its disconnect event, and `ongoing` until then.
  Workspace app, port forwarding, and tunnel events never match a `status` filter.

<a id="capturingexporting-connection-logs"></a>

## Capture and export connection logs

In addition to the Coder dashboard, there are multiple ways to consume or query
connection events.

### REST API

You can retrieve connection logs via the Coder API.
Visit the
[`get-connection-logs` endpoint documentation](../../reference/api/enterprise.md#get-connection-logs)
for details.

Each connection log in the response describes the connection with these fields:

- `connection_method`: How the connection was established, with the same values as the `method` filter.
- `app_name`: The client app that made an SSH or reconnecting PTY connection, such as `vscode`.
  The response omits this field when the client app isn't known, and for workspace app, port forwarding, and tunnel connections.
- `web_info.slug_or_port`: The workspace app slug or forwarded port, for workspace app and port forwarding connections.
  This field keeps its existing meaning.
- `type`: Deprecated.
  Use `connection_method` and `app_name` instead.

If you read `type`, map its values to the new fields as follows:

| `type`                                       | `connection_method` | `app_name`                                                    |
|----------------------------------------------|---------------------|---------------------------------------------------------------|
| `ssh`                                        | `ssh`               | Omitted, or an app outside the VS Code and JetBrains families |
| `vscode`                                     | `ssh`               | An app in the VS Code family, such as `vscode`                |
| `jetbrains`                                  | `ssh`               | An app in the JetBrains family, such as `jetbrains`           |
| `reconnecting_pty`                           | `reconnecting_pty`  | Omitted unless the client app is known                        |
| `workspace_app`, `port_forwarding`, `tunnel` | Same as `type`      | Omitted                                                       |

The `type` field keeps the same values, but the API schema describes it as a string rather than an enum.

### Service Logs

Connection events are also dispatched as service logs and can be captured and
categorized using any log management tool such as [Splunk](https://splunk.com).

Each `connection_log` entry describes the connection with these fields:

- `ConnectionMethod`: How the connection was established, with the same values as the API `connection_method` field.
- `AppNameOrPort`: The client app for SSH and reconnecting PTY connections, or the workspace app slug or forwarded port for workspace app and port forwarding connections.
  The value is an empty string when there is nothing to report.

If your log pipeline reads the `Type` or `SlugOrPort` fields, update it to read `ConnectionMethod` and `AppNameOrPort`, because entries no longer include the old fields.
For SSH connections from a VS Code or JetBrains family app, `ConnectionMethod` is `ssh` and `AppNameOrPort` holds the app name, such as `vscode`.
Every other former `Type` value matches `ConnectionMethod` directly.

Example of a [JSON formatted](../../reference/cli/server/index.md#--log-json) connection log entry, when VS Code connects over SSH:

```json
{
    "ts": "2026-10-07T11:34:37.428786242Z",
    "level": "INFO",
    "msg": "connection_log",
    "caller": "/home/coder/coder/enterprise/audit/backends/slog.go:36",
    "func": "github.com/coder/coder/v2/enterprise/audit/backends.(*SlogExporter).ExportStruct",
    "logger_names": ["coderd"],
    "fields": {
        "ID": "2e3b3337-77ab-4677-b39f-ba98df4e8ae4",
        "OrganizationID": "8654721b-2d93-4559-afdb-bdbdc2acbdc2",
        "WorkspaceOwnerID": "1a7edc8c-b712-4a1f-9d8f-7727127912e2",
        "WorkspaceID": "59d3e101-2d07-40c8-9bcd-d5fff4cc5266",
        "WorkspaceName": "dev",
        "AgentName": "main",
        "ConnectionMethod": "ssh",
        "Code": null,
        "IP": "fd7a:115c:a1e0:4b86:9046:80e:6c70:33b7",
        "UserAgent": "",
        "UserID": null,
        "AppNameOrPort": "vscode",
        "ConnectionID": "875ee185-cdf7-4cd1-98d7-d8a415e42a26",
        "DisconnectReason": "",
        "ClientSessionID": "",
        "Time": "2026-10-07T11:34:37.428771813Z",
        "ConnectionStatus": "connected"
    }
}
```

Example of a [human readable](../../reference/cli/server/index.md#--log-human) connection log entry, when a user opens `code-server`:

```txt
2026-10-07 11:34:37.428 [info]  coderd: connection_log  ID=909a0310-a14c-4761-89d3-9779a328f14f  OrganizationID=dc39157d-b298-415d-8fc7-590e9055f244  WorkspaceOwnerID=17f1cc14-ac7a-4703-976e-73ea4547dceb  WorkspaceID=f2e648d4-95bc-4df3-a5c2-ee5be991da29  WorkspaceName=dev  AgentName=main  ConnectionMethod=workspace_app  Code=200  IP=127.0.0.1  UserAgent="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36"  UserID=17f1cc14-ac7a-4703-976e-73ea4547dceb  AppNameOrPort=code-server  ConnectionID=<nil>  DisconnectReason=""  ClientSessionID=""  Time="2026-10-07 11:34:37.428773435 +0000 UTC"  ConnectionStatus=connected
```

## Data Retention

Coder supports configurable retention policies that automatically purge old
Connection Logs. To enable automated purging, configure the
`--connection-logs-retention` flag or `CODER_CONNECTION_LOGS_RETENTION`
environment variable. For comprehensive configuration options, see
[Data Retention](../setup/data-retention.md).

## How to Enable Connection Logs

This feature is only available with a [Premium license](../../install/prepare/licensing.md).
