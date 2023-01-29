## coder config-ssh

Add an SSH Host entry for your workspaces "ssh coder.workspace"

```
coder config-ssh [flags]
```

### Examples

```
  - You can use -o (or --ssh-option) so set SSH options to be used for all your
    workspaces:

      $ coder config-ssh -o ForwardAgent=yes

  - You can use --dry-run (or -n) to see the changes that would be made:

      $ coder config-ssh --dry-run
```

### Options

```
  -n, --dry-run                  Perform a trial run with no changes made, showing a diff at the end.
  -h, --help                     help for config-ssh
      --ssh-config-file string   Specifies the path to an SSH config.
                                 Consumes $CODER_SSH_CONFIG_FILE (default "~/.ssh/config")
  -o, --ssh-option stringArray   Specifies additional SSH options to embed in each host stanza.
      --use-previous-options     Specifies whether or not to keep options from previous run of config-ssh.
                                 Consumes $CODER_SSH_USE_PREVIOUS_OPTIONS
  -y, --yes                      Bypass prompts
```

### Options inherited from parent commands

```
      --global-config coder   Path to the global coder config directory.
                              Consumes $CODER_CONFIG_DIR (default "~/.config/coderv2")
      --header stringArray    HTTP headers added to all requests. Provide as "Key=Value".
                              Consumes $CODER_HEADER
      --no-feature-warning    Suppress warnings about unlicensed features.
                              Consumes $CODER_NO_FEATURE_WARNING
      --no-version-warning    Suppress warning when client and server versions do not match.
                              Consumes $CODER_NO_VERSION_WARNING
      --token string          Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
                              Consumes $CODER_SESSION_TOKEN
      --url string            URL to a deployment.
                              Consumes $CODER_URL
  -v, --verbose               Enable verbose output.
                              Consumes $CODER_VERBOSE
```

### SEE ALSO

- [coder](coder.md) -
