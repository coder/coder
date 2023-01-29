## coder scaletest

Run a scale test against the Coder API

### Synopsis

Perform scale tests against the Coder server.

```
coder scaletest [flags]
```

### Options

```
  -h, --help   help for scaletest
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
- [coder scaletest cleanup](coder_scaletest_cleanup.md) - Cleanup any orphaned scaletest resources
- [coder scaletest create-workspaces](coder_scaletest_create-workspaces.md) - Creates many workspaces and waits for them to be ready
