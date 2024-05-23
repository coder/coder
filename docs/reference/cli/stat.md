<!-- DO NOT EDIT | GENERATED CONTENT -->

# stat

Show resource usage for the current workspace.

## Usage

```console
coder stat [flags]
```

## Subcommands

| Name                                | Purpose                          |
| ----------------------------------- | -------------------------------- |
| [<code>cpu</code>](./stat_cpu.md)   | Show CPU usage, in cores.        |
| [<code>mem</code>](./stat_mem.md)   | Show memory usage, in gigabytes. |
| [<code>disk</code>](./stat_disk.md) | Show disk usage, in gigabytes.   |

## Options

### -c, --column

|         |                                                                            |
| ------- | -------------------------------------------------------------------------- |
| Type    | <code>string-array</code>                                                  |
| Default | <code>host_cpu,host_memory,home_disk,container_cpu,container_memory</code> |

Columns to display in table output. Available columns: host cpu, host memory, home disk, container cpu, container memory.

### -o, --output

|         |                     |
| ------- | ------------------- |
| Type    | <code>string</code> |
| Default | <code>table</code>  |

Output format. Available formats: table, json.
