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

Bypass prompts.

### --roles

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |

A list of roles to give to the user. This removes any existing roles the user may have. The available roles are: auditor, member, owner, template-admin, user-admin.
