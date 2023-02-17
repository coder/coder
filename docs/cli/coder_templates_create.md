## coder templates create

Create a template from the current directory or as specified by flag

```
coder templates create [name] [flags]
```

### Options

```
      --default-ttl duration          Specify a default TTL for workspaces created from this template. (default 24h0m0s)
  -d, --directory string              Specify the directory to create from, use '-' to read tar from stdin (default "<current-directory>")
  -h, --help                          help for create
      --parameter-file string         Specify a file path with parameter values.
      --provisioner-tag stringArray   Specify a set of tags to target provisioner daemons.
      --variable stringArray          Specify a set of values for Terraform-managed variables.
      --variables-file string         Specify a file path with values for Terraform-managed variables.
  -y, --yes                           Bypass prompts
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
