<!-- DO NOT EDIT | GENERATED CONTENT -->
# config-ssh

Add an SSH Host entry for your workspaces "ssh workspace.coder"

## Usage

```console
coder config-ssh [flags]
```

## Description

```console
  - You can use -o (or --ssh-option) so set SSH options to be used for all your
workspaces:

     $ coder config-ssh -o ForwardAgent=yes

  - You can use --dry-run (or -n) to see the changes that would be made:

     $ coder config-ssh --dry-run
```

## Options

### --ssh-config-file

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_SSH_CONFIG_FILE</code> |
| Default     | <code>~/.ssh/config</code>          |

Specifies the path to an SSH config.

### --coder-binary-path

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_SSH_CONFIG_BINARY_PATH</code> |

Optionally specify the absolute path to the coder binary used in ProxyCommand. By default, the binary invoking this command ('config ssh') is used.

### -o, --ssh-option

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string-array</code>           |
| Environment | <code>$CODER_SSH_CONFIG_OPTS</code> |

Specifies additional SSH options to embed in each host stanza.

### -n, --dry-run

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>bool</code>               |
| Environment | <code>$CODER_SSH_DRY_RUN</code> |

Perform a trial run with no changes made, showing a diff at the end.

### --use-previous-options

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>bool</code>                            |
| Environment | <code>$CODER_SSH_USE_PREVIOUS_OPTIONS</code> |

Specifies whether or not to keep options from previous run of config-ssh.

### --ssh-host-prefix

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>string</code>                           |
| Environment | <code>$CODER_CONFIGSSH_SSH_HOST_PREFIX</code> |

Override the default host prefix.

### --hostname-suffix

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>string</code>                           |
| Environment | <code>$CODER_CONFIGSSH_HOSTNAME_SUFFIX</code> |

Override the default hostname suffix.

### --wait

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>yes\|no\|auto</code>         |
| Environment | <code>$CODER_CONFIGSSH_WAIT</code> |
| Default     | <code>auto</code>                  |

Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.

### --disable-autostart

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>bool</code>                               |
| Environment | <code>$CODER_CONFIGSSH_DISABLE_AUTOSTART</code> |
| Default     | <code>false</code>                              |

Disable starting the workspace automatically when connecting via SSH.

### --no-wildcard

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_CONFIGSSH_NO_WILDCARD</code> |
| Default     | <code>false</code>                        |

Generate an individual host entry for each workspace instead of a wildcard host block. This allows third-party tools and SSH clients to discover workspaces by reading the config file.

### --force-unix-filepaths

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>bool</code>                            |
| Environment | <code>$CODER_CONFIGSSH_UNIX_FILEPATHS</code> |

By default, 'config-ssh' uses the os path separator when writing the ssh config. This might be an issue in Windows machine that use a unix-like shell. This flag forces the use of unix file paths (the forward slash '/').

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass confirmation prompts.
