<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates create

Create a template from the current directory or as specified by flag

## Usage

```console
coder templates create [flags] [name]
```

## Options

### --default-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>24h</code>      |

Specify a default TTL for workspaces created from this template.

### -d, --directory

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>.</code>      |

Specify the directory to create from, use '-' to read tar from stdin.

### --failure-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a failure TTL for workspaces created from this template. This licensed feature's default is 0h (off).

### --ignore-lockfile

|         |                    |
| ------- | ------------------ |
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Ignore warnings about not having a .terraform.lock.hcl file present in the template.

### --inactivity-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify an inactivity TTL for workspaces created from this template. This licensed feature's default is 0h (off).

### -m, --message

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specify a message describing the changes in this version of the template. Messages longer than 72 characters will be displayed as truncated.

### --private

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Disable the default behavior of granting template access to the 'everyone' group. The template permissions must be updated to allow non-admin users to use this template.

### --provisioner-tag

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Specify a set of tags to target provisioner daemons.

### --var

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Alias of --variable.

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
