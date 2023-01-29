## coder schedule override-stop

Edit stop time of active workspace

### Synopsis

Override the stop time of a currently running workspace instance.

- The new stop time is calculated from _now_.
- The new stop time must be at least 30 minutes in the future.
- The workspace template may restrict the maximum workspace runtime.

```
coder schedule override-stop <workspace-name> <duration from now> [flags]
```

### Examples

```
  $ coder schedule override-stop my-workspace 90m
```

### Options

```
  -h, --help   help for override-stop
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

- [coder schedule](coder_schedule.md) - Schedule automated start and stop times for workspaces
