<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates push

Push a new template version from the current directory or as specified by flag

## Usage

```console
coder templates push [template]
```

## Options

### --always-prompt

Always prompt all parameters. Does not pull parameter values from active template version.

### -d, --directory

|         |                |
| ------- | -------------- |
| Default | <code>.</code> |

Specify the directory to create from, use '-' to read tar from stdin.

### --name

Specify a name for the new template version. It will be automatically generated if not provided.

### --parameter-file

Specify a file path with parameter values.

### -t, --provisioner-tag

Specify a set of tags to target provisioner daemons.

### --variable

Specify a set of values for Terraform-managed variables.

### --variables-file

Specify a file path with values for Terraform-managed variables.

### -y, --yes

Bypass prompts.
