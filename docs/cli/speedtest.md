<!-- DO NOT EDIT | GENERATED CONTENT -->

# speedtest

Run upload and download tests from your machine to a workspace

## Usage

```console
coder speedtest [flags] <workspace>
```

## Options

### -d, --direct

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Specifies whether to wait for a direct connection before testing speed.

### --direction

|         |                             |
| ------- | --------------------------- |
| Type    | <code>enum[up\|down]</code> |
| Default | <code>down</code>           |

Specifies whether to run in reverse mode where the client receives and the server sends.

### -t, --time

|         |                       |
| ------- | --------------------- |
| Type    | <code>duration</code> |
| Default | <code>5s</code>       |

Specifies the duration to monitor traffic.
