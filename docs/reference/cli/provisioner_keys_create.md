<!-- DO NOT EDIT | GENERATED CONTENT -->
# provisioner keys create

Create a new provisioner key

## Usage

```console
coder provisioner keys create [flags] <name>
```

## Options

### -t, --tag

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_PROVISIONERD_TAGS</code> |

Tags to filter provisioner jobs by.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
