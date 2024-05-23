<!-- DO NOT EDIT | GENERATED CONTENT -->

# stat mem

Show memory usage, in gigabytes.

## Usage

```console
coder stat mem [flags]
```

## Options

### --host

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Force host memory measurement.

### --prefix

|         |                                   |
| ------- | --------------------------------- |
| Type    | <code>enum[Ki\|Mi\|Gi\|Ti]</code> |
| Default | <code>Gi</code>                   |

SI Prefix for memory measurement.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>text</code>   |

Output format. Available formats: text, json.
