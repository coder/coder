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

Only show logs for a specific build number. Defaults to 0, which maps to the most recent build (build numbers start at 1). Negative values are treated as offsets—for example, -1 refers to the previous build.

### -f, --follow

|         |                    |
|---------|--------------------|
| Type    | <code>bool</code>  |
| Default | <code>false</code> |

Follow logs as they are emitted.
