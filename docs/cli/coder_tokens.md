## coder tokens

Manage personal access tokens

### Synopsis

Tokens are used to authenticate automated clients to Coder.

```
coder tokens [flags]
```

### Examples

```
  - Create a token for automation:

      $ coder tokens create

  - List your tokens:

      $ coder tokens ls

  - Remove a token by ID:

      $ coder tokens rm WuoWs4ZsMX
```

### Options

```
  -h, --help   help for tokens
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
- [coder tokens create](coder_tokens_create.md) - Create a tokens
- [coder tokens list](coder_tokens_list.md) - List tokens
- [coder tokens remove](coder_tokens_remove.md) - Delete a token
