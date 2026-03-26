<!-- DO NOT EDIT | GENERATED CONTENT -->
# users oidc-claims

Display the OIDC claims for the authenticated user.

## Usage

```console
coder users oidc-claims [flags]
```

## Description

```console
  - Display your OIDC claims:

     $ coder users oidc-claims

  - Display your OIDC claims as JSON:

     $ coder users oidc-claims -o json
```

## Options

### -c, --column

|         |                           |
|---------|---------------------------|
| Type    | <code>[key\|value]</code> |
| Default | <code>key,value</code>    |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
