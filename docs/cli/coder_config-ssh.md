<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder config-ssh

Add an SSH Host entry for your workspaces "ssh coder.workspace"

## Usage

```console
coder config-ssh [flags]
```

## Examples

```console
  - You can use -o (or --ssh-option) so set SSH options to be used for all your
    workspaces:

      $ coder config-ssh -o ForwardAgent=yes

  - You can use --dry-run (or -n) to see the changes that would be made:

      $ coder config-ssh --dry-run
```

## Flags

### --dry-run, -n

Perform a trial run with no changes made, showing a diff at the end.
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |

### --ssh-config-file

Specifies the path to an SSH config.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_CONFIG_FILE</code> |
| Default | <code>~/.ssh/config</code> |

### --ssh-option, -o

Specifies additional SSH options to embed in each host stanza.
<br/>
| | |
| --- | --- |
| Default | <code>[]</code> |

### --use-previous-options

Specifies whether or not to keep options from previous run of config-ssh.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SSH_USE_PREVIOUS_OPTIONS</code> |
| Default | <code>false</code> |

### --yes, -y

Bypass prompts
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |
