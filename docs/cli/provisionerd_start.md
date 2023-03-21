<!-- DO NOT EDIT | GENERATED CONTENT -->

# provisionerd start

Run a provisioner daemon

## Usage

```console
coder provisionerd start
```

## Options

### --cache-dir, -c

|             |                                     |
| ----------- | ----------------------------------- |
| Environment | <code>$CODER_CACHE_DIRECTORY</code> |
| Default     | <code>~/.cache/coder</code>         |

Directory to store cached data.

### --tag, -t

|             |                                       |
| ----------- | ------------------------------------- |
| Environment | <code>$CODER_PROVISIONERD_TAGS</code> |

Tags to filter provisioner jobs by.

### --poll-interval

|             |                                                |
| ----------- | ---------------------------------------------- |
| Environment | <code>$CODER_PROVISIONERD_POLL_INTERVAL</code> |
| Default     | <code>1s</code>                                |

How often to poll for provisioner jobs.

### --poll-jitter

|             |                                              |
| ----------- | -------------------------------------------- |
| Environment | <code>$CODER_PROVISIONERD_POLL_JITTER</code> |
| Default     | <code>100ms</code>                           |

How much to jitter the poll interval by.
