<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates versions diff

Compare two versions of a template

## Usage

```console
coder templates versions diff [flags] <template>
```

## Description

```console
  - Compare two specific versions of a template:

     $ coder templates versions diff my-template --from v1 --to v2

  - Compare a version against the active version:

     $ coder templates versions diff my-template --from v1

  - Interactive: select versions to compare:

     $ coder templates versions diff my-template
```

## Options

### --from

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

The base version to compare from.

### --to

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

The target version to compare to (defaults to active version).

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
