## coder users suspend

Update a user's status to 'suspended'. A suspended user cannot log into the platform

```
coder users suspend <username|user_id> [flags]
```

### Examples

```
  $ coder users suspend example_user
```

### Options

```
  -c, --column stringArray   Specify a column to filter in the table. (default [username,email,created_at,status])
  -h, --help                 help for suspend
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
