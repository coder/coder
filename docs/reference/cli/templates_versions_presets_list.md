<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates versions presets list

List all the presets of the specified template version

## Usage

```console
coder templates versions presets list [flags] <template> <version>
```

## Options

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.

### -c, --column

|         |                                                                      |
|---------|----------------------------------------------------------------------|
| Type    | <code>[name\|parameters\|default\|desired prebuild instances]</code> |
| Default | <code>name,parameters,default,desired prebuild instances</code>      |

Columns to display in table output.

### -o, --output

|         |                          |
|---------|--------------------------|
| Type    | <code>table\|json</code> |
| Default | <code>table</code>       |

Output format.
