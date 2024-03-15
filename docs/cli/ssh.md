<!-- DO NOT EDIT | GENERATED CONTENT -->

# ssh

Start a shell into a workspace

## Usage

```console
coder ssh [flags] <workspace>
```

## Options

### --stdio

|             |                               |
| ----------- | ----------------------------- |
| Type        | <code>bool</code>             |
| Environment | <code>$CODER_SSH_STDIO</code> |

Specifies whether to emit SSH output over stdin/stdout.

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

### --workspace-poll-interval

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>duration</code>                       |
| Environment | <code>$CODER_WORKSPACE_POLL_INTERVAL</code> |
| Default     | <code>1m</code>                             |

Specifies how often to poll for workspace automated shutdown.

### --wait

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>enum[yes\|no\|auto]</code> |
| Environment | <code>$CODER_SSH_WAIT</code>     |
| Default     | <code>auto</code>                |

Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.

### --no-wait

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>bool</code>               |
| Environment | <code>$CODER_SSH_NO_WAIT</code> |

Enter workspace immediately after the agent has connected. This is the default if the template has configured the agent startup script behavior as non-blocking.

### -l, --log-dir

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>string</code>             |
| Environment | <code>$CODER_SSH_LOG_DIR</code> |

Specify the directory containing SSH diagnostic log files.

### -R, --remote-forward

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string-array</code>              |
| Environment | <code>$CODER_SSH_REMOTE_FORWARD</code> |

Enable remote port forwarding (remote_port:local_address:local_port).

### --disable-autostart

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_SSH_DISABLE_AUTOSTART</code> |
| Default     | <code>false</code>                        |

Disable starting the workspace automatically when connecting via SSH.
