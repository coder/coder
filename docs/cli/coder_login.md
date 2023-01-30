## coder login

Authenticate with Coder deployment

```
coder login <url> [flags]
```

### Options

```
      --first-user-email string      Specifies an email address to use if creating the first user for the deployment.
                                     Consumes $CODER_FIRST_USER_EMAIL
      --first-user-password string   Specifies a password to use if creating the first user for the deployment.
                                     Consumes $CODER_FIRST_USER_PASSWORD
      --first-user-trial             Specifies whether a trial license should be provisioned for the Coder deployment or not.
                                     Consumes $CODER_FIRST_USER_TRIAL
      --first-user-username string   Specifies a username to use if creating the first user for the deployment.
                                     Consumes $CODER_FIRST_USER_USERNAME
  -h, --help                         help for login
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
