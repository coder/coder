---
title: Trace workspace connections
---

This guide is for Coder deployment administrators and platform teams who need to correlate workspace connection logs with client session IDs.
It covers `coder ssh`, VS Code, JetBrains Toolbox, and the web terminal.
You'll collect logs from the user's client, `coderd` (the process that runs the control plane), and the workspace agent.
Then you'll match entries by session ID and read the connection and workspace lifecycle fields.
For connection log filters and export formats, refer to [Connection logs](./connection-logs.md).
To diagnose a connection that is still open, refer to [Networking troubleshooting](../networking/troubleshooting.md).

## Before you begin

- Update the Coder CLI and workspace agents to the same version as your control plane.
  Agents from earlier versions keep working, but they don't record client session IDs, app names, workspace shutdown reasons, or bytes transferred.
- Collect `coderd` logs with a log management tool such as Splunk, Datadog, or Grafana Loki.
  JSON output is best for querying by other programs, such as `jq`.
  To write JSON logs, set [`CODER_LOGGING_JSON`](../../reference/cli/server/index.md#--log-json) to a file path, such as `/dev/stderr`.
  Human-readable logs also go to `/dev/stderr` by default, so set [`CODER_LOGGING_HUMAN`](../../reference/cli/server/index.md#--log-human) to `/dev/null` to keep only JSON there.
- The `connection_log` entries in this guide require a [Premium license](https://coder.com/pricing#compare-plans).
  Client and agent logs don't.
- For VS Code, use the Coder Remote extension version 1.17.2 or later.
- For JetBrains Toolbox, use the Coder Toolbox plugin version 0.9.5 or later to send session IDs and to include `coder ssh` logs in **Collect logs**.
  Plugin version 0.9.5 requires JetBrains Toolbox version 3.9.0 or later.

## How client session IDs work

Each supported client creates a client session ID, a 32-character lowercase hexadecimal value.
The client writes the ID to its own logs and sends it with its requests to `coderd` and the workspace agent.
`coderd`, workspace proxies, and the agent add the same ID to their log lines as `client_session_id`.
The agent also records it in the connection log as `ClientSessionID` for SSH sessions, web terminal connections, and JetBrains Toolbox forwarded-channel connections.
Search each log source for one value to follow a session from the client, through `coderd`, to the agent.

What counts as one session depends on the client:

| Client                                                                 | One session covers                                                       |
|------------------------------------------------------------------------|--------------------------------------------------------------------------|
| `coder ssh`, including OpenSSH connections that use `coder config-ssh` | One command run.                                                         |
| VS Code                                                                | One VS Code window. Reloading the window starts a new session.           |
| JetBrains Toolbox                                                      | One workspace agent that Toolbox connects to.                            |
| Web terminal                                                           | One page load. Reloading the page or opening a new tab starts a new one. |

VS Code and JetBrains Toolbox pass their session ID to the `coder ssh` process they start, so the IDE log, the CLI log, and the server-side logs share one ID.
To correlate your own tooling the same way, set `CODER_TRACE_SESSION_ID` to a 32-character lowercase hexadecimal value before you run `coder`.
`coderd` and the agent ignore values in any other format.

One client session often spans several SSH sessions or web terminal connections.
For example, an IDE that reconnects after a network interruption opens a new SSH connection each time, and one SSH connection can carry several SSH sessions.
Each SSH session or web terminal connection has its own `ConnectionID`, shared by its `connected` and `disconnected` connection log entries, and the same `ClientSessionID` as the rest of the client session.

## Find the session ID

Ask the user for their client logs from the session, then find the `client_session_id` field.
If the user can't provide them, start from the connection log for their workspace and time window instead, as described in [Correlate a session](#correlate-a-session).

### `coder ssh`

`coder ssh` writes one log file per session.
The default directory depends on the operating system:

| Operating system | Default directory                                                        |
|------------------|--------------------------------------------------------------------------|
| Linux            | `~/.local/state/coder/logs`, or `$XDG_STATE_HOME/coder/logs` if it's set |
| macOS            | `~/Library/Application Support/coder/logs`                               |
| Windows          | `%LOCALAPPDATA%\coder\logs`                                              |

File names start with `coder-ssh-` followed by the date and time the session started, for example `coder-ssh-20261013-141502-abcde.log`.
When `coder ssh` runs as an OpenSSH proxy command, the name also ends with the parent process ID.
Before it creates a new file, the CLI deletes all but the 50 most recently modified `coder-*.log` files in the directory.
If removal fails, the CLI logs a warning and continues the session.
To write them somewhere else, set [`--log-dir`](../../reference/cli/ssh.md#-l---log-dir) or `CODER_SSH_LOG_DIR`.

Every line in the file includes `client_session_id`.
By default, the file records info-level entries.
The CLI also keeps the 10 most recent debug entries in memory and writes them to the file when the command logs an error.
To keep more, set `--flight-recorder-size` or `CODER_FLIGHT_RECORDER_SIZE`.
To record every debug entry, set `--verbose` or `CODER_VERBOSE=true`.

### VS Code

The extension starts `coder ssh` with debug logging and passes it the session ID.
You can get the session ID from VS Code in three ways:

- **Support bundle**: Ask the user to run **Coder: Create Support Bundle** from the Command Palette.
  If the workspace agent is reachable, the bundle contains the agent log in `agent/logs.txt`.
  Under `vscode-logs/`, it contains the `coder ssh` logs, the extension logs with the **Coder** output channel, and the Remote - SSH logs.
  For more details, refer to [Generate a support bundle](../../support/support-bundle.md#vs-code).
- **`coder ssh` log directory**: The extension writes the `coder ssh` logs to the directory in the `coder.proxyLogDirectory` setting.
  If the setting is empty, it uses `CODER_SSH_LOG_DIR`, and if neither is set, a `log` directory in the extension's global storage.
  For the file names and contents, refer to [`coder ssh`](#coder-ssh).
- **Coder output channel**: Each line starts with `[client_session_id=<session-id>]`, with the full 32-character session ID.
  The extension keeps the 1,000 most recent entries below the channel's log level and writes them to the channel when a connection fails or the user collects a support bundle.
  To change the number of entries, set `coder.connectionLogBuffer.size`.
  If the window reloaded or closed after the failure, the channel no longer shows the earlier output.
  The editor also saves the channel as `Coder.log` in its logs folder, and the location of that folder depends on the editor, such as VS Code or Cursor.
  In VS Code, run **Developer: Open Logs Folder** from the Command Palette.

### JetBrains Toolbox

Toolbox starts `coder ssh`.
Toolbox log lines for a session include the full 32-character `client_session_id`.
With Coder Toolbox plugin version 0.9.5 or later, the plugin first checks the SSH log directory in the Coder plugin settings.
If that setting is empty, it uses `CODER_SSH_LOG_DIR`.
If neither is set, it uses an `ssh-logs` directory in the plugin's data directory for the deployment.

Ask the user to select **Collect logs** from the workspace action menu.
With plugin version 0.9.5 or later, successful collection includes Toolbox logs and a nested Coder support bundle, `coder-support.zip`.
The nested bundle contains recent `coder ssh` logs under `toolbox-logs/ssh/`.
If Coder bundle collection fails, the plugin attempts to include the SSH logs in `coder-ssh.zip` and collection details in `coder-support-error.txt`.

With earlier plugin versions, **Collect logs** includes only the Toolbox logs.
These versions write `coder ssh` logs to the [`coder ssh` log directory](#coder-ssh), unless an SSH log directory is set in the plugin settings.
Ask the user for those files separately and [generate a support bundle with the CLI](../../support/support-bundle.md#generate-with-the-cli).
For details, refer to [Generate a support bundle](../../support/support-bundle.md) and [JetBrains Toolbox troubleshooting](../../user-guides/workspace-access/jetbrains/toolbox.md#troubleshooting).

### Web terminal

The web terminal logs connection errors, including `client_session_id`, to the browser's developer console.
If the user didn't keep the console output, find the session in the agent logs or the connection log by workspace and time.

## Collect server-side logs

Gather `coderd` and agent logs that cover the session's time window.

### `coderd` logs

`coderd` writes a `connection_log` entry at the info level each time a connection opens or closes, so these entries don't require debug logging.
For SSH sessions and web terminal connections, the agent reports both events, so each one produces a `connected` entry and a `disconnected` entry with the same `ConnectionID`.
JetBrains Toolbox sessions work differently, as described in [Agent logs](#agent-logs).
The fields that matter most for tracing are in the following table:

| Field                | Description                                                                                                                                            |
|----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ClientSessionID`    | The client session ID. Empty for workspace app, port forwarding, and tunnel events, and for connections from older clients or agents.                  |
| `ConnectionID`       | Identifies an SSH session, web terminal connection, or tracked JetBrains forwarded channel. For SSH sessions, matches the `id` field in the agent log. |
| `ConnectionStatus`   | `connected` or `disconnected`.                                                                                                                         |
| `ConnectionMethod`   | How the connection was made, such as `ssh` or `reconnecting_pty`.                                                                                      |
| `AppNameOrPort`      | The app that made an SSH or web terminal connection, such as `vscode` or `cursor`.                                                                     |
| `DisconnectReason`   | Why the connection closed, on `disconnected` entries. Refer to [Read disconnect reasons](#read-disconnect-reasons).                                    |
| `RxBytes`, `TxBytes` | Bytes the agent received and sent over an SSH or web terminal connection. Set only on `disconnected` entries, and empty for older agents.              |

For every field and an example entry, refer to [Service logs](./connection-logs.md#service-logs).

`coderd` and workspace proxies also add `client_session_id` to request logs for API requests and path-based workspace app requests.
Requests that fail with a `5xx` status are logged at the warn level.
All other requests, including `4xx` failures such as `401`, `403`, and `404`, are logged at the debug level, so they appear only when debug logging is on.
To turn it on, set [`CODER_LOG_FILTER`](../../reference/cli/server/index.md#-l---log-filter) to `.*`, which greatly increases log volume.

### Agent logs

The workspace agent writes its log to `coder-agent.log` in the directory set by `CODER_AGENT_LOG_DIR`.
The agent rotates the file at 5&nbsp;MB and keeps up to 19 rotated files named `coder-agent-<timestamp>.log` in the same directory.
If `CODER_AGENT_LOG_DIR` isn't set, the agent uses the system temporary directory.
On Linux and macOS, that's `$TMPDIR` if it's set, or `/tmp` otherwise.
On Windows, it's the user's temporary directory, usually `%TEMP%`.
In Kubernetes workspaces, the same entries are also in the pod logs.
To collect the agent log without workspace access, run `coder support bundle <owner>/<workspace>` and open `agent/logs.txt`.
For details, refer to [Generate a support bundle](../../support/support-bundle.md).

By default, the agent logs at the info level.
It keeps the 1,000 most recent debug entries in memory and writes them to the log when it loses its connection to `coderd` or before it logs an error.
To change the number of entries, set `CODER_AGENT_FLIGHT_RECORDER_SIZE` in the agent's environment.
A value of `0` turns it off.
To record every debug entry, set `CODER_VERBOSE=true` in the agent's environment.

Search the agent log for these messages:

| Message                                    | What it records                                                                                                                                       |
|--------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `handling ssh session`                     | An SSH session started.                                                                                                                               |
| `ssh session closed`                       | An SSH session ended. Coder includes `disconnect_reason`, `disconnect_expected`, and `exit_code`.                                                     |
| `ssh session failed`                       | An SSH session failed with an error. The error text becomes the `disconnect_reason`.                                                                  |
| `ssh connection complete`                  | An SSH connection closed. If the agent was shutting down, Coder also includes `disconnect_reason`, `disconnect_expected`, and `disconnect_initiator`. |
| `ssh connection failed`                    | An SSH connection failed before a session started.                                                                                                    |
| `accepted conn`, `reconnecting pty closed` | A web terminal connection opened or closed, from the `reconnecting-pty` logger. The closing entry includes `disconnect_reason`.                       |
| `disconnected from coderd`                 | The agent lost its connection to `coderd`. The buffered debug entries are written just before this line.                                              |

The `speedtest` logger also writes `accepted conn` for `coder speedtest` connections.
Use the logger name to distinguish those entries from web terminal connections.

JetBrains Toolbox opens many short SSH sessions, so `handling ssh session` lines are frequent for Toolbox users.
For those sessions, the agent doesn't log `ssh session closed` or report them to the connection log.
Instead, it reports the forwarded channel that Toolbox keeps open as one connection, and logs `JetBrains watcher accepted channel` and `JetBrains channel closed` at the debug level.

## Correlate a session

With a session ID and the logs, rebuild the timeline of the session.

> [!TIP]
> To see every connection in the session from the dashboard, open the **Connection Log** page, open an entry's details, and select **View related connections**.
> The page lists the connections in the same workspace that share the client session ID.
> The details also show the **Client session ID** with a copy button, which you can use to search the `coderd` and agent logs.

1. Search the `coderd` logs for `connection_log` entries where the `ClientSessionID` matches the session ID.

   For example, with JSON logs in a file:

   ```sh
   jq -c 'select(.msg == "connection_log" and .fields.ClientSessionID == "<session-id>")' coderd.log
   ```

   Each entry names the workspace, agent, connection method, app, and `ConnectionID`.

1. Note the time of each `connected` and `disconnected` entry and its `DisconnectReason`.

1. Search the `coderd` logs for `client_session_id` set to the same value.

   Warn-level entries show `5xx` responses.
   Debug logs also include `4xx` responses, as described in [`coderd` logs](#coderd-logs).

1. Search the agent log for the session ID:

   On Linux and macOS:

   ```sh
   grep '<session-id>' <agent-log-dir>/coder-agent*.log
   ```

   On Windows workspaces, use PowerShell:

   ```ps1
   Select-String -Path '<agent-log-dir>\coder-agent*.log' -Pattern '<session-id>' -SimpleMatch
   ```

   The matching lines show each SSH or web terminal session the client opened and the reason it closed.

1. Line up the client log with the server-side entries by time.

   Matching timestamps and session IDs connect the client activity to the reported connection events.

A client session ID doesn't guarantee a corresponding `connection_log` entry.
Connections that don't reach the agent and earlier agents that don't record session IDs can leave no matching entries.
Reports can also be lost if the agent's reporting buffer fills or a report to the control plane fails.
The agent logs `connection report buffer limit reached, dropping report` or `failed to report connection to server` for those reporting failures.

If you start without a session ID, filter the connection log by `workspace_owner` or `workspace_id`.
Use `connected_after` and `connected_before` with RFC3339 timestamps to limit connection start times.
Both time boundaries are inclusive.
Then take `ClientSessionID` from the corresponding `connection_log` service entries.

## Read disconnect reasons

When an SSH session or web terminal connection closes, the agent logs `disconnect_reason` and `disconnect_expected` and reports the reason to the connection log as `DisconnectReason`.
The two can differ after a workspace stop or delete: the agent logs `server_shutdown`, and `coderd` records `workspace_stopped` in the connection log.
`coderd` records `workspace_stopped` only when a stop or delete build that was requested before the connection closed is running or has succeeded.
Otherwise, the connection log keeps `server_shutdown`.

While a stop or delete build is pending or running, `coderd` keeps the workspace agent connected so the agent can report why its connections closed and `coderd` can look up the build.
As a result, the agent still shows as connected while the workspace is stopping.

| `disconnect_reason` | Meaning                                                                                                                                              |
|---------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `graceful`          | The connection ended cleanly, for example when the SSH command exited with status `0`.                                                               |
| `network_error`     | A web terminal connection returned a setup or attachment error, or the agent lost its connection to `coderd`. SSH sessions don't record this reason. |
| `server_shutdown`   | The agent closed the connection because the agent was shutting down.                                                                                 |
| `workspace_stopped` | A workspace stop or delete closed the connection. Only the connection log and the `coder ssh` log record this reason.                                |
| `unknown`           | The agent couldn't determine why the connection closed. Only the agent log shows `unknown`; in the connection log, `DisconnectReason` is empty.      |
| `normal close`      | A JetBrains Toolbox forwarded channel closed while the agent was running. Only the connection log records this reason.                               |

For web terminal connections, `network_error` includes transport errors and errors creating the terminal command.
The agent also records it on `disconnected from coderd`, with `connect_type=server_to_agent`.

For SSH sessions, the reason can also be a specific message, such as `process exited with error status: 1` when the command exited with a nonzero status, or the raw error text when the session failed.
SSH sessions don't record `network_error`.
If the agent was shutting down when an SSH session ended, the reason is `server_shutdown`, and the agent log keeps the specific message in `disconnect_detail`.

In the agent log, `disconnect_expected` is `true` for `graceful` and `server_shutdown`.
It's `false` for `network_error`, `unknown`, and specific error messages, including nonzero remote-command exit statuses.
A value of `false` can describe an ordinary command failure, not a network failure.
`connection_log` entries don't include `disconnect_expected`.

When a workspace stop or delete ends a `coder ssh` session, the `coder ssh` log has a `workspace is shutting down` line with `disconnect_reason` set to `workspace_stopped`, `build_id`, `build_reason`, and `transition`.
The agent log doesn't include these build fields.
`build_reason` shows what stopped the workspace:

| `build_reason` | The workspace stopped because                                                                                            |
|----------------|--------------------------------------------------------------------------------------------------------------------------|
| `initiator`    | A user or API client requested the build.                                                                                |
| `autostop`     | The workspace reached its autostop deadline, its owner was suspended, or a failed start or stop build triggered cleanup. |
| `dormancy`     | The workspace became dormant from inactivity.                                                                            |
| `autodelete`   | Coder deleted the dormant workspace automatically, based on the template's schedule.                                     |

## Learn more

- [Connection logs](./connection-logs.md): filter, export, and retain connection events
- [Generate a support bundle](../../support/support-bundle.md): collect client, agent, and deployment diagnostics in one archive
- [Networking troubleshooting](../networking/troubleshooting.md): diagnose direct and relayed connections with `coder ping`
