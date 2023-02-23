<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder ssh

Start a shell into a workspace

## Usage

```console
coder ssh <workspace> [flags]
```

## Flags

### --forward-agent, -A

Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_FORWARD_AGENT</code> |
| Default | <code>false</code> |

### --forward-gpg, -G

Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpgconf) on both the client and workspace. The GPG agent must already be running locally and will not be started for you. If a GPG agent is already running in the workspace, it will be attempted to be killed.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_FORWARD_GPG</code> |
| Default | <code>false</code> |

### --identity-agent

Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_IDENTITY_AGENT</code> |

### --no-wait

Specifies whether to wait for a workspace to become ready before logging in (only applicable when the login before ready option has not been enabled). Note that the workspace agent may still be in the process of executing the startup script and the workspace may be in an incomplete state.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_NO_WAIT</code> |
| Default | <code>false</code> |

### --stdio

Specifies whether to emit SSH output over stdin/stdout.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_STDIO</code> |
| Default | <code>false</code> |

### --workspace-poll-interval

Specifies how often to poll for workspace automated shutdown.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_WORKSPACE_POLL_INTERVAL</code> |
| Default | <code>1m0s</code> |
