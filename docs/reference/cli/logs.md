<!-- DO NOT EDIT | GENERATED CONTENT -->
# logs

View logs for a workspace

## Usage

```console
coder logs [flags] <workspace>
```

## Description

```console
View logs for a workspace
```

## Options

### -n, --build-number

|         |                  |
|---------|------------------|
| Type    | <code>int</code> |
| Default | <code>0</code>   |

Only show logs for a specific build number. Defaults to the most recent build. If a negative number is provided, it is treated as an offset from the most recent build. For example, -1 would refer to the previous build.

### -f, --follow

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Follow logs as they are emitted.
