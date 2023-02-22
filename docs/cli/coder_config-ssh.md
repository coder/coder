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

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --dry-run, -n | false | <code>Perform a trial run with no changes made, showing a diff at the end.</code>|
| --skip-proxy-command | false | <code>Specifies whether the ProxyCommand option should be skipped. Useful for testing.</code>|
| --ssh-config-file | ~/.ssh/config | <code>Specifies the path to an SSH config.<br/>Consumes $CODER_SSH_CONFIG_FILE</code>|
| --ssh-option, -o | [] | <code>Specifies additional SSH options to embed in each host stanza.</code>|
| --use-previous-options | false | <code>Specifies whether or not to keep options from previous run of config-ssh.<br/>Consumes $CODER_SSH_USE_PREVIOUS_OPTIONS</code>|
| --yes, -y | false | <code>Bypass prompts</code>|