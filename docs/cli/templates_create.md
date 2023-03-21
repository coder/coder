<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates create

Create a template from the current directory or as specified by flag

## Usage

```console
coder templates create [name]
```

## Options

### --default-ttl

|         |                  |
| ------- | ---------------- |
| Default | <code>24h</code> |

Specify a default TTL for workspaces created from this template.

### -d, --directory

|         |                |
| ------- | -------------- |
| Default | <code>.</code> |

Specify the directory to create from, use '-' to read tar from stdin.

### --parameter-file

Specify a file path with parameter values.

### --provisioner-tag

Specify a set of tags to target provisioner daemons.

### --variable

Specify a set of values for Terraform-managed variables.

### --variables-file

Specify a file path with values for Terraform-managed variables.

### -y, --yes

Bypass prompts.
