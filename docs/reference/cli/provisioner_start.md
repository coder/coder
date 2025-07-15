<!-- DO NOT EDIT | GENERATED CONTENT -->
# provisioner start

Run a provisioner daemon

## Usage

```console
coder provisioner start [flags]
```

## Options

### -c, --cache-dir

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_CACHE_DIRECTORY</code> |
| Default     | <code>~/.cache/coder</code>         |

Directory to store cached data.

### -t, --tag

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_PROVISIONERD_TAGS</code> |

Tags to filter provisioner jobs by.

### --poll-interval

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>duration</code>                          |
| Environment | <code>$CODER_PROVISIONERD_POLL_INTERVAL</code> |
| Default     | <code>1s</code>                                |

Deprecated and ignored.

### --poll-jitter

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>duration</code>                        |
| Environment | <code>$CODER_PROVISIONERD_POLL_JITTER</code> |
| Default     | <code>100ms</code>                           |

Deprecated and ignored.

### --psk

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMON_PSK</code> |

Pre-shared key to authenticate with Coder server.

### --key

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string</code>                        |
| Environment | <code>$CODER_PROVISIONER_DAEMON_KEY</code> |

Provisioner key to authenticate with Coder server.

### --name

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_PROVISIONER_DAEMON_NAME</code> |

Name of this provisioner daemon. Defaults to the current hostname without FQDN.

### --verbose

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_PROVISIONER_DAEMON_VERBOSE</code> |
| Default     | <code>false</code>                             |

Output debug-level logs.

### --log-human

|             |                                                      |
|-------------|------------------------------------------------------|
| Type        | <code>string</code>                                  |
| Environment | <code>$CODER_PROVISIONER_DAEMON_LOGGING_HUMAN</code> |
| Default     | <code>/dev/stderr</code>                             |

Output human-readable logs to a given file.

### --log-json

|             |                                                     |
|-------------|-----------------------------------------------------|
| Type        | <code>string</code>                                 |
| Environment | <code>$CODER_PROVISIONER_DAEMON_LOGGING_JSON</code> |

Output JSON logs to a given file.

### --log-stackdriver

|             |                                                            |
|-------------|------------------------------------------------------------|
| Type        | <code>string</code>                                        |
| Environment | <code>$CODER_PROVISIONER_DAEMON_LOGGING_STACKDRIVER</code> |

Output Stackdriver compatible logs to a given file.

### --log-filter

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>string-array</code>                         |
| Environment | <code>$CODER_PROVISIONER_DAEMON_LOG_FILTER</code> |

Filter debug logs by matching against a given regex. Use .* to match all debug logs.

### --prometheus-enable

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>bool</code>                     |
| Environment | <code>$CODER_PROMETHEUS_ENABLE</code> |
| Default     | <code>false</code>                    |

Serve prometheus metrics on the address defined by prometheus address.

### --prometheus-address

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_PROMETHEUS_ADDRESS</code> |
| Default     | <code>127.0.0.1:2112</code>            |

The bind address to serve prometheus metrics.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
