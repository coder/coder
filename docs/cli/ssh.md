<!-- DO NOT EDIT | GENERATED CONTENT -->

# ssh

Start a shell into a workspace

## Usage

```console
coder ssh [flags] <workspace>
```

## Options

### -A, --forward-agent

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>bool</code>                     |
| Environment | <code>$CODER_SSH_FORWARD_AGENT</code> |

Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK.

### -G, --forward-gpg

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_SSH_FORWARD_GPG</code> |

Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpgconf) on both the client and workspace. The GPG agent must already be running locally and will not be started for you. If a GPG agent is already running in the workspace, it will be attempted to be killed.

### --identity-agent

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_SSH_IDENTITY_AGENT</code> |

Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled.

### --log-dir

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>string</code>             |
| Environment | <code>$CODER_SSH_LOG_DIR</code> |
| Default     | <code>/tmp</code>               |

Specify the location for the log files.

### -l, --log-to-file

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_SSH_LOG_TO_FILE</code> |

Enable diagnostic logging to file.

### --no-wait

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>bool</code>               |
| Environment | <code>$CODER_SSH_NO_WAIT</code> |

Specifies whether to wait for a workspace to become ready before logging in (only applicable when the startup script behavior is blocking). Note that the workspace agent may still be in the process of executing the startup script and the workspace may be in an incomplete state.

### --stdio

|             |                               |
| ----------- | ----------------------------- |
| Type        | <code>bool</code>             |
| Environment | <code>$CODER_SSH_STDIO</code> |

Specifies whether to emit SSH output over stdin/stdout.

### --workspace-poll-interval

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>duration</code>                       |
| Environment | <code>$CODER_WORKSPACE_POLL_INTERVAL</code> |
| Default     | <code>1m</code>                             |

Specifies how often to poll for workspace automated shutdown.
