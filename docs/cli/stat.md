<!-- DO NOT EDIT | GENERATED CONTENT -->

# stat

Show workspace resource usage.

## Usage

```console
coder stat [flags]
```

## Options

### -c, --column

|         |                                                                            |
| ------- | -------------------------------------------------------------------------- |
| Type    | <code>string-array</code>                                                  |
| Default | <code>container_cpu,container_memory,host_cpu,host_memory,home_disk</code> |

Columns to display in table output. Available columns: host cpu, host memory, home disk, container cpu, container memory.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>table</code>  |

Output format. Available formats: table, json.
