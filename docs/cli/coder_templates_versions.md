## coder templates versions

Manage different versions of the specified template

```
coder templates versions [flags]
```

### Examples

```
  - List versions of a specific template:

      $ coder templates versions list my-template
```

### Options

```
  -h, --help   help for versions
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

- [coder templates](coder_templates.md) - Manage templates
- [coder templates versions list](coder_templates_versions_list.md) - List all the versions of the specified template
