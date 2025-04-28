<!-- DO NOT EDIT | GENERATED CONTENT -->
# organizations show

Show the organization. Using "selected" will show the selected organization from the "--org" flag. Using "me" will show all organizations you are a member of.

## Usage

```console
coder organizations show [flags] ["selected"|"me"|uuid|org_name]
```

## Description

```console
  - coder org show selected:

     $ Shows the organizations selected with '--org=<org_name>'. This organization is the organization used by the cli.

  - coder org show me:

     $ List of all organizations you are a member of.

  - coder org show developers:

     $ Show organization with name 'developers'

  - coder org show 90ee1875-3db5-43b3-828e-af3687522e43:

     $ Show organization with the given ID.
```

## Options

### --only-id

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Only print the organization ID.

### -c, --column

|         |                                                                                           |
|---------|-------------------------------------------------------------------------------------------|
| Type    | <code>[id\|name\|display name\|icon\|description\|created at\|updated at\|default]</code> |
| Default | <code>id,name,default</code>                                                              |

Columns to display in table output.

### -o, --output

|         |                                |
|---------|--------------------------------|
| Type    | <code>text\|table\|json</code> |
| Default | <code>text</code>              |

Output format.
