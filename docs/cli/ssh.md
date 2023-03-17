<!-- DO NOT EDIT | GENERATED CONTENT -->

# ssh

Start a shell into a workspace

## Usage

```console
ssh <workspace>
```

## Options

### --stdio

|             |                               |
| ----------- | ----------------------------- |
| Environment | <code>$CODER_SSH_STDIO</code> |

Specifies whether to emit SSH output over stdin/stdout.

### --shuffle

|             |                                 |
| ----------- | ------------------------------- |
| Environment | <code>$CODER_SSH_SHUFFLE</code> |

Specifies whether to choose a random workspace.

### --forward-agent, -A

|             |                                       |
| ----------- | ------------------------------------- |
| Environment | <code>$CODER_SSH_FORWARD_AGENT</code> |

Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK.

### --forward-gpg, -G

|             |                                     |
| ----------- | ----------------------------------- |
| Environment | <code>$CODER_SSH_FORWARD_GPG</code> |

Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpg2, gpgsm) and gpg-agent to be installed on the host.

### --identity-agent

|             |                                        |
| ----------- | -------------------------------------- |
| Environment | <code>$CODER_SSH_IDENTITY_AGENT</code> |

Specifies the path to the SSH agent socket to use for identity forwarding. Defaults to $SSH_AUTH_SOCK.

### --workspace-poll-interval

|             |                                             |
| ----------- | ------------------------------------------- |
| Environment | <code>$CODER_WORKSPACE_POLL_INTERVAL</code> |

Specifies the interval at which to poll for a workspace to be ready. Defaults to 1s.

### --no-wait

|             |                                 |
| ----------- | ------------------------------- |
| Environment | <code>$CODER_SSH_NO_WAIT</code> |

Specifies whether to wait for the workspace to be ready before connecting. Defaults to false.
