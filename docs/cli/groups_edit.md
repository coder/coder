<!-- DO NOT EDIT | GENERATED CONTENT -->

# groups edit

Edit a user group

## Usage

```console
coder groups edit [flags] <name>
```

## Options

### -a, --add-users

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Add users to the group. Accepts emails or IDs.

### -u, --avatar-url

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Update the group avatar.

### --display-name

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string</code>              |
| Environment | <code>$CODER_DISPLAY_NAME</code> |

Optional human friendly name for the group.

### -n, --name

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Update the group name.

### -r, --rm-users

|      |                           |
| ---- | ------------------------- |
| Type | <code>string-array</code> |

Remove users to the group. Accepts emails or IDs.
