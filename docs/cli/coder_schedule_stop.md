## coder schedule stop

Edit workspace stop schedule

### Synopsis

Schedules a workspace to stop after a given duration has elapsed.

- Workspace runtime is measured from the time that the workspace build completed.
- The minimum scheduled stop time is 1 minute.
- The workspace template may place restrictions on the maximum shutdown time.
- Changes to workspace schedules only take effect upon the next build of the workspace,
  and do not affect a running instance of a workspace.

When enabling scheduled stop, enter a duration in one of the following formats:

- 3h2m (3 hours and two minutes)
- 3h (3 hours)
- 2m (2 minutes)
- 2 (2 minutes)

```
coder schedule stop <workspace-name> { <duration> | manual } [flags]
```

### Examples

```
  $ coder schedule stop my-workspace 2h30m
```

### Options

```
  -h, --help   help for stop
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
