<!-- DO NOT EDIT | GENERATED CONTENT -->

# stat disk

Show disk usage, in gigabytes.

## Usage

```console
coder stat disk [flags]
```

## Options

### --path

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>/</code>      |

Path for which to check disk usage.

### --prefix

|         |                                   |
| ------- | --------------------------------- |
| Type    | <code>enum[Ki\|Mi\|Gi\|Ti]</code> |
| Default | <code>Gi</code>                   |

SI Prefix for disk measurement.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>text</code>   |

Output format. Available formats: text, json.
