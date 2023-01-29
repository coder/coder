## coder schedule

Schedule automated start and stop times for workspaces

```
coder schedule { show | start | stop | override } <workspace> [flags]
```

### Options

```
  -h, --help   help for schedule
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
- [coder schedule override-stop](coder_schedule_override-stop.md) - Edit stop time of active workspace
- [coder schedule show](coder_schedule_show.md) - Show workspace schedule
- [coder schedule start](coder_schedule_start.md) - Edit workspace start schedule
- [coder schedule stop](coder_schedule_stop.md) - Edit workspace stop schedule
