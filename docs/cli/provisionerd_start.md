<!-- DO NOT EDIT | GENERATED CONTENT -->

# provisionerd start

Run a provisioner daemon

## Usage

```console
coder provisionerd start [flags]
```

## Options

### -c, --cache-dir

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_CACHE_DIRECTORY</code> |
| Default     | <code>~/.cache/coder</code>         |

Directory to store cached data.

### --log-filter

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string-array</code>                   |
| Environment | <code>$CODER_PROVISIONERD_LOG_FILTER</code> |

Filter debug logs by matching against a given regex. Use .\* to match all debug logs.

### --log-human

|             |                                            |
| ----------- | ------------------------------------------ |
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PROVISIONERD_LOG_HUMAN</code> |
| Default     | <code>/dev/stderr</code>                   |

Log in human-readable format to the given path.

### --log-json

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_PROVISIONERD_LOG_JSON</code> |

Log in JSON format to the given path.

### --log-stackdriver

|             |                                                  |
| ----------- | ------------------------------------------------ |
| Type        | <code>string</code>                              |
| Environment | <code>$CODER_PROVISIONERD_LOG_STACKDRIVER</code> |

Log in Stackdriver format to the given path.

### --name

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_PROVISIONER_DAEMON_NAME</code> |

Name of this provisioner daemon. Defaults to the current hostname without FQDN.

### --poll-interval

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>duration</code>                          |
| Environment | <code>$CODER_PROVISIONERD_POLL_INTERVAL</code> |
| Default     | <code>1s</code>                                |

Deprecated and ignored.

### --poll-jitter

|             |                                              |
| ----------- | -------------------------------------------- |
| Type        | <code>duration</code>                        |
| Environment | <code>$CODER_PROVISIONERD_POLL_JITTER</code> |
| Default     | <code>100ms</code>                           |

Deprecated and ignored.

### --psk

|             |                                            |
| ----------- | ------------------------------------------ |
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMON_PSK</code> |

Pre-shared key to authenticate with Coder server.

### -t, --tag

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_PROVISIONERD_TAGS</code> |

Tags to filter provisioner jobs by.

### --verbose

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_PROVISIONERD_VERBOSE</code> |
| Default     | <code>false</code>                       |

Enable verbose logging. This is useful for debugging, but can be noisy when running in production.
