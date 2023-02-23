<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder scaletest create-workspaces

Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.

It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.

## Usage

```console
coder scaletest create-workspaces [flags]
```

## Flags

### --cleanup-concurrency

Number of concurrent cleanup jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_CONCURRENCY</code> |
| Default | <code>1</code> |

### --cleanup-job-timeout

Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_JOB_TIMEOUT</code> |
| Default | <code>5m0s</code> |

### --cleanup-timeout

Timeout for the entire cleanup run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_TIMEOUT</code> |
| Default | <code>30m0s</code> |

### --concurrency

Number of concurrent jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONCURRENCY</code> |
| Default | <code>1</code> |

### --connect-hold

How long to hold the WireGuard connection open for.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONNECT_HOLD</code> |
| Default | <code>30s</code> |

### --connect-interval

How long to wait between making requests to the --connect-url once the connection is established.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONNECT_INTERVAL</code> |
| Default | <code>1s</code> |

### --connect-mode

Mode to use for connecting to the workspace. Can be 'derp' or 'direct'.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONNECT_MODE</code> |
| Default | <code>derp</code> |

### --connect-timeout

Timeout for each request to the --connect-url.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONNECT_TIMEOUT</code> |
| Default | <code>5s</code> |

### --connect-url

URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CONNECT_URL</code> |

### --count, -c

Required: Number of workspaces to create.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_COUNT</code> |
| Default | <code>1</code> |

### --job-timeout

Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_JOB_TIMEOUT</code> |
| Default | <code>5m0s</code> |

### --no-cleanup

Do not clean up resources after the test completes. You can cleanup manually using `coder scaletest cleanup`.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_NO_CLEANUP</code> |
| Default | <code>false</code> |

### --no-plan

Skip the dry-run step to plan the workspace creation. This step ensures that the given parameters are valid for the given template.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_NO_PLAN</code> |
| Default | <code>false</code> |

### --no-wait-for-agents

Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_NO_WAIT_FOR_AGENTS</code> |
| Default | <code>false</code> |

### --output

Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SCALETEST_OUTPUTS</code> |
| Default | <code>[text]</code> |

### --parameter

Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_PARAMETERS</code> |
| Default | <code>[]</code> |

### --parameters-file

Path to a YAML file containing the parameters to use for each workspace.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_PARAMETERS_FILE</code> |

### --run-command

Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_RUN_COMMAND</code> |

### --run-expect-output

Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_RUN_EXPECT_OUTPUT</code> |

### --run-expect-timeout

Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_RUN_EXPECT_TIMEOUT</code> |
| Default | <code>false</code> |

### --run-log-output

Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_RUN_LOG_OUTPUT</code> |
| Default | <code>false</code> |

### --run-timeout

Timeout for the command to complete.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_RUN_TIMEOUT</code> |
| Default | <code>5s</code> |

### --template, -t

Required: Name or ID of the template to use for workspaces.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TEMPLATE</code> |

### --timeout

Timeout for the entire test run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TIMEOUT</code> |
| Default | <code>30m0s</code> |

### --trace

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TRACE</code> |
| Default | <code>false</code> |

### --trace-coder

Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TRACE_CODER</code> |
| Default | <code>false</code> |

### --trace-honeycomb-api-key

Enables trace exporting to Honeycomb.io using the provided API key.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TRACE_HONEYCOMB_API_KEY</code> |

### --trace-propagate

Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_TRACE_PROPAGATE</code> |
| Default | <code>false</code> |
