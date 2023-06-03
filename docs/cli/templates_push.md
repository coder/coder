<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates push

Push a new template version from the current directory or as specified by flag

## Usage

```console
coder templates push [flags] [template]
```

## Options

### --activate

|         |                   |
| ------- | ----------------- |
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Whether the new template will be marked active.

### --always-prompt

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Always prompt all parameters. Does not pull parameter values from active template version.

### -d, --directory

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>.</code>      |

Specify the directory to create from, use '-' to read tar from stdin.

### --name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specify a name for the new template version. It will be automatically generated if not provided.

### --provisioner-tag

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Specify a set of tags to target provisioner daemons.

### --variable

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Specify a set of values for Terraform-managed variables.

### --variables-file

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specify a file path with values for Terraform-managed variables.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
