<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates create

DEPRECATED: Create a template from the current directory or as specified by flag

## Usage

```console
coder templates create [flags] [name]
```

## Options

### --private

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Disable the default behavior of granting template access to the 'everyone' group. The template permissions must be updated to allow non-admin users to use this template.

### --variables-file

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specify a file path with values for Terraform-managed variables.

### --variable

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Specify a set of values for Terraform-managed variables.

### --var

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Alias of --variable.

### --provisioner-tag

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Specify a set of tags to target provisioner daemons.

### --default-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>24h</code>      |

Specify a default TTL for workspaces created from this template. It is the default time before shutdown - workspaces created from this template default to this value. Maps to "Default autostop" in the UI.

### --failure-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a failure TTL for workspaces created from this template. It is the amount of time after a failed "start" build before coder automatically schedules a "stop" build to cleanup.This licensed feature's default is 0h (off). Maps to "Failure cleanup"in the UI.

### --dormancy-threshold

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a duration workspaces may be inactive prior to being moved to the dormant state. This licensed feature's default is 0h (off). Maps to "Dormancy threshold" in the UI.

### --dormancy-auto-deletion

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a duration workspaces may be in the dormant state prior to being deleted. This licensed feature's default is 0h (off). Maps to "Dormancy Auto-Deletion" in the UI.

### --max-ttl

|      |                       |
| ---- | --------------------- |
| Type | <code>duration</code> |

Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.

### --require-active-version

|         |                    |
| ------- | ------------------ |
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Requires workspace builds to use the active template version. This setting does not apply to template admins. This is an enterprise-only feature.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.

### -d, --directory

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>.</code>      |

Specify the directory to create from, use '-' to read tar from stdin.

### --ignore-lockfile

|         |                    |
| ------- | ------------------ |
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Ignore warnings about not having a .terraform.lock.hcl file present in the template.

### -m, --message

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specify a message describing the changes in this version of the template. Messages longer than 72 characters will be displayed as truncated.
