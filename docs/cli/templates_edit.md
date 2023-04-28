<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates edit

Edit the metadata of a template by name.

## Usage

```console
coder templates edit [flags] <template>
```

## Options

### --allow-user-autostart

|         |                   |
| ------- | ----------------- |
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Allow users to configure autostart for workspaces on this template. This can only be disabled in enterprise.

### --allow-user-autostop

|         |                   |
| ------- | ----------------- |
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Allow users to customize the autostop TTL for workspaces on this template. This can only be disabled in enterprise.

### --allow-user-cancel-workspace-jobs

|         |                   |
| ------- | ----------------- |
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Allow users to cancel in-progress workspace jobs.

### --default-ttl

|      |                       |
| ---- | --------------------- |
| Type | <code>duration</code> |

Edit the template default time before shutdown - workspaces created from this template default to this value.

### --description

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template description.

### --display-name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template display name.

### --icon

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template icon path.

### --max-ttl

|      |                       |
| ---- | --------------------- |
| Type | <code>duration</code> |

Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.

### --name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template name.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
