<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates push

Create or update a template from the current directory or as specified by flag

## Usage

```console
coder templates push [flags] [template]
```

## Options

### --variables-file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Specify a file path with values for Terraform-managed variables.

### --variable

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Specify a set of values for Terraform-managed variables.

### --var

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Alias of --variable.

### --provisioner-tag

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Specify a set of tags to target provisioner daemons.

### --name

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Specify a name for the new template version. It will be automatically generated if not provided.

### --always-prompt

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Always prompt all parameters. Does not pull parameter values from active template version.

### --activate

|         |                   |
|---------|-------------------|
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Whether the new template will be marked active.

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### -d, --directory

|         |                     |
|---------|---------------------|
| Type    | <code>string</code> |
| Default | <code>.</code>      |

Specify the directory to create from, use '-' to read tar from stdin.

### --ignore-lockfile

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Ignore warnings about not having a .terraform.lock.hcl file present in the template.

### -m, --message

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Specify a message describing the changes in this version of the template. Messages longer than 72 characters will be displayed as truncated.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
