# Connection Logs

Connection Logs allows **Auditors** to monitor workspace agent connections.

## Workspace app connections

The connection log contains a complete record of all **workspace app** connections.
These originate from within the Coder deployment, and thus the connection log
is a source of truth for these events.

## Browser port forwarding

The connection log contains a complete record of all workspace port forwarding
performed via the web dashboard.

## SSH & IDE sessions

The connection log aims to capture a record of SSH & IDE sessions to workspaces.
These events are reported by workspace agents, and their receipt by the server
is not guaranteed.

## Filtering logs

Connection logs can be filtered with the following parameters:

- `organization` - The name or ID of the organization of the workspace being
     connected to.
- `workspace_owner` - The username of the owner of the workspace being connected
    to.
- `type` - The type of the connection (i.e. SSH, VSCode, Workspace App).
    An exhaustive list is
    [available here](https://pkg.go.dev/github.com/coder/coder/v2/codersdk#ConnectionType).
- `username`: The name of the user who initiated the connection.
- `user_email`: The email of the user who initiated the connection.
- `started_after`: The time after which the connection started. Uses the RFC3339Nano format.
- `started_before`: The time before which the connection started. Uses the RFC3339Nano format.
- `workspace_id`: The ID of the workspace being connected to.
- `connection_id`: The ID of the connection.
- `status`: The status of the connection, either `ongoing` or `completed`.
     Some events are neither ongoing nor completed, such as the opening of a
     workspace app.

## Capturing/Exporting Connection Logs

In addition to the user interface, there are multiple ways to consume or query
connection events.

## REST API

Connection logs can be retrieved via our REST API. You can find detailed
information about this in our
[endpoint documentation](../../reference/api/enterprise.md#get-connection-logs).

## Service Logs

Connection events are also dispatched as service logs and can be captured and
categorized using any log management tool such as [Splunk](https://splunk.com).

Example of a [JSON formatted](../../reference/cli/server.md#--log-json)
connection log entry, when an SSH connection is made:

```json
{
    "ts": "2025-07-03T05:09:41.929840747Z",
    "level": "INFO",
    "msg": "connection_log",
    "caller": "/home/coder/coder/enterprise/audit/backends/slog.go:38",
    "func": "github.com/coder/coder/v2/enterprise/audit/backends.(*SlogExporter).ExportStruct",
    "logger_names": ["coderd"],
    "fields": {
        "request_id": "2bd88dd5-f7a5-4e29-b5ba-543400798c8c",
        "ID": "b4f043e3-2010-4dd2-a1fe-a7c2c923e236",
        "Time": "2025-07-03T05:09:41.923468875Z",
        "OrganizationID": "0665a54f-0b77-4a58-94aa-59646fa38a74",
        "WorkspaceOwnerID": "6dea5f8c-ecec-4cf0-a5bd-bc2c63af2efa",
        "WorkspaceID": "3c0b37c8-e58c-4980-b9a1-2732410480a5",
        "WorkspaceName": "dev",
        "AgentName": "main",
        "Type": "ssh",
        "Code": null,
        "Ip": "fd7a:115c:a1e0:4afd:8ffb:2fc9:4b5:da61",
        "UserAgent": "",
        "UserID": null,
        "SlugOrPort": "",
        "ConnectionID": "dcfe943f-2afb-4e3f-8f00-3eb1718a3f8b",
        "CloseReason": "",
        "ConnectionStatus": "connected"
    }
}
```

Example of a [human readable](../../reference/cli/server.md#--log-human)
connection log entry, when `code-server` is opened:

```console
[API] 2025-07-03 06:57:16.157 [info]  coderd: connection_log  request_id=de3f6004-6cc1-4880-a296-d7c6ca1abf75  ID=f0249951-d454-48f6-9504-e73340fa07b7  Time="2025-07-03T06:57:16.144719Z"  OrganizationID=0665a54f-0b77-4a58-94aa-59646fa38a74  WorkspaceOwnerID=6dea5f8c-ecec-4cf0-a5bd-bc2c63af2efa  WorkspaceID=3c0b37c8-e58c-4980-b9a1-2732410480a5  WorkspaceName=dev  AgentName=main  Type=workspace_app  Code=200  Ip=127.0.0.1  UserAgent="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36"  UserID=6dea5f8c-ecec-4cf0-a5bd-bc2c63af2efa  SlugOrPort=code-server  ConnectionID=<nil>  CloseReason=""  ConnectionStatus=connected
```

## Enabling this feature

This feature is only available with a premium license.
