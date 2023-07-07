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

### --failure-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a failure TTL for workspaces created from this template. This licensed feature's default is 0h (off).

### --icon

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template icon path.

### --inactivity-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify an inactivity TTL for workspaces created from this template. This licensed feature's default is 0h (off).

### --name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template name.

### --restart-requirement-weekdays

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Edit the template restart requirement weekdays - workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the restart requirement for the template), pass 'none'.

### --restart-requirement-weeks

|      |                  |
| ---- | ---------------- |
| Type | <code>int</code> |

Edit the template restart requirement weeks - workspaces created from this template must be restarted on an n-weekly basis.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
