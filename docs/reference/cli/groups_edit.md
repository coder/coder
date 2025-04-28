<!-- DO NOT EDIT | GENERATED CONTENT -->
# groups edit

Edit a user group

## Usage

```console
coder groups edit [flags] <name>
```

## Options

### -n, --name

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the group name.

### -u, --avatar-url

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Update the group avatar.

### --display-name

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_DISPLAY_NAME</code> |

Optional human friendly name for the group.

### -a, --add-users

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Add users to the group. Accepts emails or IDs.

### -r, --rm-users

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

Remove users to the group. Accepts emails or IDs.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
