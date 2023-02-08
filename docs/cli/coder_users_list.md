## coder users list

```
coder users list [flags]
```

### Options

```
  -c, --column strings   Columns to display in table output. Available columns: id, username, email, created at, status (default [username,email,created_at,status])
  -h, --help             help for list
  -o, --output string    Output format. Available formats: table, json (default "table")
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

- [coder users](coder_users.md) - Manage users
