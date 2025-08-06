<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates presets list

List all presets of the specified template. Defaults to the active template version.

## Usage

```console
coder templates presets list [flags] <template>
```

## Options

### --template-version

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Specify a template version to list presets for. Defaults to the active version.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                                   |
|---------|-----------------------------------------------------------------------------------|
| Type    | <code>[name\|description\|parameters\|default\|desired prebuild instances]</code> |
| Default | <code>name,description,parameters,default,desired prebuild instances</code>       |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
