<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates edit

Edit the metadata of a template by name.

## Usage

```console
coder templates edit [flags] <template>
```

## Options

### --name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template name.

### --display-name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template display name.

### --description

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template description.

### --deprecated

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Sets the template as deprecated. Must be a message explaining why the template is deprecated.

### --icon

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Edit the template icon path.

### --default-ttl

|      |                       |
| ---- | --------------------- |
| Type | <code>duration</code> |

Edit the template default time before shutdown - workspaces created from this template default to this value. Maps to "Default autostop" in the UI.

### --activity-bump

|      |                       |
| ---- | --------------------- |
| Type | <code>duration</code> |

Edit the template activity bump - workspaces created from this template will have their shutdown time bumped by this value when activity is detected. Maps to "Activity bump" in the UI.

### --autostart-requirement-weekdays

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Edit the template autostart requirement weekdays - workspaces created from this template can only autostart on the given weekdays. To unset this value for the template (and allow autostart on all days), pass 'all'.

### --autostop-requirement-weekdays

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Edit the template autostop requirement weekdays - workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the autostop requirement for the template), pass 'none'.

### --autostop-requirement-weeks

|      |                  |
| ---- | ---------------- |
| Type | <code>int</code> |

Edit the template autostop requirement weeks - workspaces created from this template must be restarted on an n-weekly basis.

### --failure-ttl

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>0h</code>       |

Specify a failure TTL for workspaces created from this template. It is the amount of time after a failed "start" build before coder automatically schedules a "stop" build to cleanup.This licensed feature's default is 0h (off). Maps to "Failure cleanup" in the UI.

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

### --allow-user-cancel-workspace-jobs

|         |                   |
| ------- | ----------------- |
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Allow users to cancel in-progress workspace jobs.

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

### --require-active-version

|         |                    |
| ------- | ------------------ |
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Requires workspace builds to use the active template version. This setting does not apply to template admins. This is an enterprise-only feature.

### --private

|         |                    |
| ------- | ------------------ |
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Disable the default behavior of granting template access to the 'everyone' group. The template permissions must be updated to allow non-admin users to use this template.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
