<!-- DO NOT EDIT | GENERATED CONTENT -->
# users edit-roles

Edit a user's roles by username or id

## Usage

```console
coder users edit-roles [flags] <username|user_id>
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass confirmation prompts.

### --roles

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

A list of roles to give to the user. This replaces all existing roles. Use --add or --remove to modify roles incrementally.

### --add

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

A list of roles to add to the user's existing roles. Cannot be used together with --roles.

### --remove

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

A list of roles to remove from the user's existing roles. Cannot be used together with --roles.
