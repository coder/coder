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

Specify a set of tags to target provisioner daemons. If you do not specify any tags, the tags from the active template version will be reused, if available. To remove existing tags, use --provisioner-tag="-".

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

Bypass confirmation prompts.

### -d, --directory

|         |                     |
|---------|---------------------|
| Type    | <code>string</code> |
| Default | <code>.</code>      |

Specify the directory to create from, use '-' to read tar from stdin.

### --follow-symlinks

|         |                    |
|---------|---------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Follow symlinks when archiving the template directory. Symlinked files and directories will be included as regular files in the archive. Symlinks that point outside the template directory are skipped.

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
