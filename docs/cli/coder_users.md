## coder users

Manage users

```
coder users [flags]
```

### Options

```
  -h, --help   help for users
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
- [coder users activate](coder_users_activate.md) - Update a user's status to 'active'. Active users can fully interact with the platform
- [coder users create](coder_users_create.md) -
- [coder users list](coder_users_list.md) -
- [coder users show](coder_users_show.md) - Show a single user. Use 'me' to indicate the currently authenticated user.
- [coder users suspend](coder_users_suspend.md) - Update a user's status to 'suspended'. A suspended user cannot log into the platform
