---
title: Trace workspace connections
---

This guide is for Coder deployment administrators and platform teams who investigate dropped or failed workspace connections from `coder ssh`, VS Code, JetBrains Toolbox, and the web terminal.
It shows how to collect logs from the user's client, the Coder control plane (`coderd`), and the workspace agent; correlate them with a client session ID; and read the fields that explain why a connection ended.
For connection log filters and export formats, refer to [Connection logs](./connection-logs.md).
To diagnose a connection that is still open, refer to [Networking troubleshooting](../networking/troubleshooting.md).

## Before you begin

- Update workspace agents to the same version as your Coder server.
  Agents from earlier versions keep working, but they don't record client session IDs, app names, workspace shutdown reasons, or bytes transferred.
- Collect `coderd` logs with a log management tool such as Splunk, Datadog, or Grafana Loki.
  JSON output is easiest to query.
  Turn on JSON output with [`CODER_LOGGING_JSON`](../../reference/cli/server/index.md#--log-json).
- The `connection_log` entries in this guide require a [Premium license](https://coder.com/pricing#compare-plans).
  Client and agent logs don't.
- For VS Code, use the Coder Remote extension version 1.16.1 or later to send session IDs, and version 1.17.0 or later to report the editor name, such as `cursor`.
- For JetBrains Toolbox, use the Coder Toolbox plugin version 0.9.5 or later to send session IDs and to include `coder ssh` logs in **Collect logs**.

## How client session IDs work

Each supported client creates a client session ID, a 32-character lowercase hexadecimal value.
The client writes the ID to its own logs and sends it with its requests to `coderd` and the workspace agent.
`coderd`, workspace proxies, and the agent add the same ID to their log lines as `client_session_id`.
The agent also records it in the connection log as `ClientSessionID` for SSH and web terminal connections.
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

One client session often spans several connections.
For example, an IDE that reconnects after a network interruption opens a new SSH connection each time.
Each connection gets its own connection log entry, with its own `ConnectionID` and the same `ClientSessionID`.

## Find the session ID

Ask the user for their client logs from the time of the failure, then find the `client_session_id` field.
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
The CLI keeps the 50 most recent files.
To write them somewhere else, set [`--log-dir`](../../reference/cli/ssh.md#-l---log-dir) or `CODER_SSH_LOG_DIR`.

Every line in the file includes `client_session_id`.
By default, the file records info-level entries.
The CLI also keeps the 10 most recent debug entries in memory and writes them to the file when the command logs an error.
To keep more, set `--flight-recorder-size` or `CODER_FLIGHT_RECORDER_SIZE`.
To record every debug entry, set `--verbose` or `CODER_VERBOSE=true`.

### VS Code

Ask the user to run **Coder: Create Support Bundle** from the Command Palette.
The bundle includes the extension logs and the `coder ssh` logs under `vscode-logs/`.
For details, refer to [Generate a support bundle](../../support/support-bundle.md#vs-code).

The extension starts `coder ssh` with debug logging.
It writes the `coder ssh` logs to the directory in the `coder.proxyLogDirectory` VS Code setting.
If the setting is empty, the extension uses `CODER_SSH_LOG_DIR`, and if that isn't set either, a `log` directory in the extension's global storage.
In the **Coder** output channel, each line starts with `[session` and the first 8 characters of the session ID.
The extension keeps the 1,000 most recent entries below the channel's log level and writes them to the channel when a connection fails or the user collects a support bundle.
To change the number of entries, set `coder.connectionLogBuffer.size`.

### JetBrains Toolbox

Toolbox starts `coder ssh`.
With Coder Toolbox plugin version 0.9.5 or later, the plugin chooses the directory for the `coder ssh` logs: the SSH log directory in the Coder plugin settings, then `CODER_SSH_LOG_DIR`, then an `ssh-logs` directory in the plugin's data directory for the deployment.
Ask the user to select **Collect logs** from the workspace action menu.
The archive includes the Toolbox logs and a Coder support bundle, and version 0.9.5 or later adds recent `coder ssh` logs to the bundle under `toolbox-logs/ssh/`.
With earlier plugin versions, `coder ssh` writes to the [`coder ssh` log directory](#coder-ssh) unless an SSH log directory is set in the plugin settings, and **Collect logs** doesn't include those files, so ask the user for them separately.
For details, refer to [JetBrains Toolbox troubleshooting](../../user-guides/workspace-access/jetbrains/toolbox.md#troubleshooting).

### Web terminal

The web terminal logs connection errors, including `client_session_id`, to the browser's developer console.
If the user didn't keep the console output, find the session in the agent logs or the connection log by workspace and time.

## Collect server-side logs

Gather `coderd` and agent logs that cover the time of the failure.

### `coderd` logs

`coderd` writes a `connection_log` entry at the info level each time a connection opens or closes, so these entries don't require debug logging.
For SSH and web terminal connections, the agent reports both events, so each connection produces a `connected` entry and a `disconnected` entry with the same `ConnectionID`.
The fields that matter most for tracing are in the following table:

| Field                | Description                                                                                                                               |
|----------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| `ClientSessionID`    | The client session ID. Empty for workspace app, port forwarding, and tunnel events, and for connections from older clients or agents.     |
| `ConnectionID`       | Identifies one SSH or web terminal connection.                                                                                            |
| `ConnectionStatus`   | `connected` or `disconnected`.                                                                                                            |
| `ConnectionMethod`   | How the connection was made, such as `ssh` or `reconnecting_pty`.                                                                         |
| `AppNameOrPort`      | The app that made an SSH or web terminal connection, such as `vscode` or `cursor`.                                                        |
| `DisconnectReason`   | Why the connection closed, on `disconnected` entries. Refer to [Read disconnect reasons](#read-disconnect-reasons).                       |
| `RxBytes`, `TxBytes` | Bytes the agent received and sent over an SSH or web terminal connection. Set only on `disconnected` entries, and empty for older agents. |

For every field and an example entry, refer to [Service logs](./connection-logs.md#service-logs).

`coderd` and workspace proxies also add `client_session_id` to request logs for API requests and path-based workspace app requests.
Failed requests with a `5xx` status are logged at the warn level.
Successful requests are logged at the debug level, so they appear only when debug logging is on.
To turn it on, set [`CODER_LOG_FILTER`](../../reference/cli/server/index.md#-l---log-filter) to `.*`, which greatly increases log volume.

### Agent logs

The workspace agent writes its log to `coder-agent.log` in the directory set by `CODER_AGENT_LOG_DIR`.
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
| `ssh connection complete`                  | An SSH connection closed. If the agent was shutting down, Coder also includes `disconnect_reason`, `disconnect_expected`, and `disconnect_initiator`. |
| `ssh connection failed`                    | An SSH connection failed before a session started.                                                                                                    |
| `accepted conn`, `reconnecting pty closed` | A web terminal connection opened or closed. The closing entry includes `disconnect_reason`.                                                           |
| `disconnected from coderd`                 | The agent lost its connection to `coderd`. The buffered debug entries are written just before this line.                                              |

## Correlate a session

With a session ID and the logs, rebuild the timeline of the session:

1. Search the `coderd` logs for `connection_log` entries where the `ClientSessionID` matches the session ID.

   For example, with JSON logs in a file:

   ```sh
   jq -c 'select(.msg == "connection_log" and .fields.ClientSessionID == "<session-id>")' coderd.log
   ```

   Each matching entry names the workspace, the agent, the connection method and app, and the `ConnectionID`.
   Note the time of each `connected` and `disconnected` entry and its `DisconnectReason`.

1. Search the `coderd` logs for `client_session_id` set to the same value.
   Matching warn-level entries show failed requests from the client.

1. Search the agent log for the session ID:

   ```sh
   grep '<session-id>' <agent-log-dir>/coder-agent.log
   ```

   The matching lines show each SSH or web terminal session the client opened and the reason it closed.

1. Line up the client log with the server-side entries by time.
   The last client entries before a disconnect usually show the error the client hit.

If the client log has the session ID but there are no `connection_log` entries for it, either the connection didn't reach the agent or the agent is older than your Coder server.
Check the `coderd` logs for the session ID, and filter the connection log with `method:tunnel` for the same workspace and time to find denied tunnel requests.

If you start without a session ID, filter the connection log by `workspace_owner`, `workspace_id`, and time, then take `ClientSessionID` from the matching service log entries.

## Read disconnect reasons

When an SSH session or web terminal connection closes, the agent logs `disconnect_reason` and `disconnect_expected` and reports the reason to the connection log as `DisconnectReason`.
The two can differ after a workspace stop or delete: the agent logs `server_shutdown`, and `coderd` records `workspace_stopped` in the connection log.
`coderd` records `workspace_stopped` only when a stop or delete build that was requested before the connection closed is running or has succeeded.
Otherwise, the connection log keeps `server_shutdown`.

| `disconnect_reason` | Meaning                                                                                                               |
|---------------------|-----------------------------------------------------------------------------------------------------------------------|
| `graceful`          | The connection ended cleanly, for example when the SSH command exited with status `0`.                                |
| `network_error`     | The transport between the client and the agent failed, for example with an EOF, a read or write error, or a timeout.  |
| `server_shutdown`   | The agent closed the connection because the agent was shutting down.                                                  |
| `workspace_stopped` | A workspace stop or delete closed the connection. Only the connection log and the `coder ssh` log record this reason. |
| `unknown`           | The agent couldn't determine why the connection closed.                                                               |

For SSH sessions, the reason can also be a specific message, such as `process exited with error status: 1` when the command exited with a nonzero status.

`disconnect_expected` is `true` for `graceful`, `server_shutdown`, and `workspace_stopped`, and `false` for every other reason.
Filter on `disconnect_expected=false` to find the disconnects worth investigating.

When a workspace stop or delete ends a `coder ssh` session, the `coder ssh` log has a `workspace is shutting down` line with `disconnect_reason` set to `workspace_stopped`, `build_id`, `build_reason`, and `transition`.
The agent log doesn't include these build fields.
`build_reason` shows what stopped the workspace:

| `build_reason` | The workspace stopped because                 |
|----------------|-----------------------------------------------|
| `initiator`    | A user or API client requested the build.     |
| `autostop`     | The workspace reached its autostop schedule.  |
| `dormancy`     | The workspace became dormant from inactivity. |

## Next steps

- [Connection logs](./connection-logs.md): filter, export, and retain connection events.
- [Generate a support bundle](../../support/support-bundle.md): collect client, agent, and deployment diagnostics in one archive.
- [Networking troubleshooting](../networking/troubleshooting.md): diagnose direct and relayed connections with `coder ping`.
