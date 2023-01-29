## coder templates

Manage templates

### Synopsis

Templates are written in standard Terraform and describe the infrastructure for workspaces

```
coder templates [flags]
```

### Examples

```
  - Create a template for developers to create workspaces:

      $ coder templates create

  - Make changes to your template, and plan the changes:

      $ coder templates plan my-template

  - Push an update to the template. Your developers can update their workspaces:

      $ coder templates push my-template
```

### Options

```
  -h, --help   help for templates
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
- [coder templates create](coder_templates_create.md) - Create a template from the current directory or as specified by flag
- [coder templates delete](coder_templates_delete.md) - Delete templates
- [coder templates edit](coder_templates_edit.md) - Edit the metadata of a template by name.
- [coder templates init](coder_templates_init.md) - Get started with a templated template.
- [coder templates list](coder_templates_list.md) - List all the templates available for the organization
- [coder templates plan](coder_templates_plan.md) - Plan a template push from the current directory
- [coder templates pull](coder_templates_pull.md) - Download the latest version of a template to a path.
- [coder templates push](coder_templates_push.md) - Push a new template version from the current directory or as specified by flag
- [coder templates versions](coder_templates_versions.md) - Manage different versions of the specified template
