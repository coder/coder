<!-- DO NOT EDIT | GENERATED CONTENT -->

# scaletest workspace-traffic

Generate traffic to scaletest workspaces through coderd

## Usage

```console
coder scaletest workspace-traffic [flags]
```

## Options

### --bytes-per-tick

|             |                                                                |
| ----------- | -------------------------------------------------------------- |
| Type        | <code>int</code>                                               |
| Environment | <code>$CODER_SCALETEST_WORKSPACE_TRAFFIC_BYTES_PER_TICK</code> |
| Default     | <code>1024</code>                                              |

How much traffic to generate per tick.

### --cleanup-concurrency

|             |                                                   |
| ----------- | ------------------------------------------------- |
| Type        | <code>int</code>                                  |
| Environment | <code>$CODER_SCALETEST_CLEANUP_CONCURRENCY</code> |
| Default     | <code>1</code>                                    |

Number of concurrent cleanup jobs to run. 0 means unlimited.

### --cleanup-job-timeout

|             |                                                   |
| ----------- | ------------------------------------------------- |
| Type        | <code>duration</code>                             |
| Environment | <code>$CODER_SCALETEST_CLEANUP_JOB_TIMEOUT</code> |
| Default     | <code>5m</code>                                   |

Timeout per job. Jobs may take longer to complete under higher concurrency limits.

### --cleanup-timeout

|             |                                               |
| ----------- | --------------------------------------------- |
| Type        | <code>duration</code>                         |
| Environment | <code>$CODER_SCALETEST_CLEANUP_TIMEOUT</code> |
| Default     | <code>30m</code>                              |

Timeout for the entire cleanup run. 0 means unlimited.

### --concurrency

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>int</code>                          |
| Environment | <code>$CODER_SCALETEST_CONCURRENCY</code> |
| Default     | <code>1</code>                            |

Number of concurrent jobs to run. 0 means unlimited.

### --job-timeout

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>duration</code>                     |
| Environment | <code>$CODER_SCALETEST_JOB_TIMEOUT</code> |
| Default     | <code>5m</code>                           |

Timeout per job. Jobs may take longer to complete under higher concurrency limits.

### --output

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string-array</code>             |
| Environment | <code>$CODER_SCALETEST_OUTPUTS</code> |
| Default     | <code>text</code>                     |

Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.

### --scaletest-prometheus-address

|             |                                                  |
| ----------- | ------------------------------------------------ |
| Type        | <code>string</code>                              |
| Environment | <code>$CODER_SCALETEST_PROMETHEUS_ADDRESS</code> |
| Default     | <code>0.0.0.0:21112</code>                       |

Address on which to expose scaletest Prometheus metrics.

### --scaletest-prometheus-wait

|             |                                               |
| ----------- | --------------------------------------------- |
| Type        | <code>duration</code>                         |
| Environment | <code>$CODER_SCALETEST_PROMETHEUS_WAIT</code> |
| Default     | <code>5s</code>                               |

How long to wait before exiting in order to allow Prometheus metrics to be scraped.

### --tick-interval

|             |                                                               |
| ----------- | ------------------------------------------------------------- |
| Type        | <code>duration</code>                                         |
| Environment | <code>$CODER_SCALETEST_WORKSPACE_TRAFFIC_TICK_INTERVAL</code> |
| Default     | <code>100ms</code>                                            |

How often to send traffic.

### --timeout

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>duration</code>                 |
| Environment | <code>$CODER_SCALETEST_TIMEOUT</code> |
| Default     | <code>30m</code>                      |

Timeout for the entire test run. 0 means unlimited.

### --trace

|             |                                     |
| ----------- | ----------------------------------- |
| Type        | <code>bool</code>                   |
| Environment | <code>$CODER_SCALETEST_TRACE</code> |

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

### --trace-coder

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_SCALETEST_TRACE_CODER</code> |

Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.

### --trace-honeycomb-api-key

|             |                                                       |
| ----------- | ----------------------------------------------------- |
| Type        | <code>string</code>                                   |
| Environment | <code>$CODER_SCALETEST_TRACE_HONEYCOMB_API_KEY</code> |

Enables trace exporting to Honeycomb.io using the provided API key.

### --trace-propagate

|             |                                               |
| ----------- | --------------------------------------------- |
| Type        | <code>bool</code>                             |
| Environment | <code>$CODER_SCALETEST_TRACE_PROPAGATE</code> |

Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.
