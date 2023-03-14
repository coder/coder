
# create-workspaces

 
Creates many workspaces and waits for them to be ready


## Usage
```console
create-workspaces
```

## Description
```console
Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.

It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.
```


## Options
### --count, -c
Required: Number of workspaces to create.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Required: Number of workspaces to create.&lt;/code&gt; |
| Default |     &lt;code&gt;1&lt;/code&gt; |



### --template, -t
Required: Name or ID of the template to use for workspaces.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Required: Name or ID of the template to use for workspaces.&lt;/code&gt; |

### --parameters-file
Path to a YAML file containing the parameters to use for each workspace.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to a YAML file containing the parameters to use for each workspace.&lt;/code&gt; |

### --parameter
Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value&lt;/code&gt; |

### --no-plan, -n
Do not print a plan of the load test before running it.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Do not print a plan of the load test before running it.&lt;/code&gt; |

### --no-cleanup
Do not clean up workspaces after the load test has finished. Useful for debugging.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Do not clean up workspaces after the load test has finished. Useful for debugging.&lt;/code&gt; |

### --no-wait-for-agents
Do not wait for agents to be ready before starting the load test. Useful for debugging.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Do not wait for agents to be ready before starting the load test. Useful for debugging.&lt;/code&gt; |

### --run-command
Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.&lt;/code&gt; |

### --run-timeout
Timeout for the command to complete.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout for the command to complete.&lt;/code&gt; |
| Default |     &lt;code&gt;5s&lt;/code&gt; |



### --run-expect-timeout
Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --run-expect-output
Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.&lt;/code&gt; |

### --run-log-output
Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.&lt;/code&gt; |

### --connect-url
URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.&lt;/code&gt; |

### --connect-mode
WireGuard connection mode. Must be one of: derp, udp, tcp.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;WireGuard connection mode. Must be one of: derp, udp, tcp.&lt;/code&gt; |
| Default |     &lt;code&gt;derp&lt;/code&gt; |



### --connect-hold
Time to hold the WireGuard connection open for.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Time to hold the WireGuard connection open for.&lt;/code&gt; |
| Default |     &lt;code&gt;30s&lt;/code&gt; |



### --connect-interval

<br/>
| | |
| --- | --- |
| Default |     &lt;code&gt;1s&lt;/code&gt; |



### --connect-timeout
Timeout for the WireGuard connection to complete.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout for the WireGuard connection to complete.&lt;/code&gt; |
| Default |     &lt;code&gt;5s&lt;/code&gt; |



### --trace
Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md&lt;/code&gt; |

### --trace-coder
Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.&lt;/code&gt; |

### --trace-honeycomb-api-key
Enables trace exporting to Honeycomb.io using the provided API key.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enables trace exporting to Honeycomb.io using the provided API key.&lt;/code&gt; |

### --trace-propagate
Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.&lt;/code&gt; |

### --concurrency
Number of concurrent jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Number of concurrent jobs to run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;1&lt;/code&gt; |



### --timeout
Timeout for the entire test run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout for the entire test run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;30m&lt;/code&gt; |



### --job-timeout
Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout per job. Jobs may take longer to complete under higher concurrency limits.&lt;/code&gt; |
| Default |     &lt;code&gt;5m&lt;/code&gt; |



### --cleanup-concurrency
Number of concurrent cleanup jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Number of concurrent cleanup jobs to run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;1&lt;/code&gt; |



### --cleanup-timeout
Timeout for the entire cleanup run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout for the entire cleanup run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;30m&lt;/code&gt; |



### --cleanup-job-timeout
Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout per job. Jobs may take longer to complete under higher concurrency limits.&lt;/code&gt; |
| Default |     &lt;code&gt;5m&lt;/code&gt; |



### --output
Output format specs in the format &#34;&lt;format&gt;[:&lt;path&gt;]&#34;. Not specifying a path will default to stdout. Available formats: text, json.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output format specs in the format &#34;&lt;format&gt;[:&lt;path&gt;]&#34;. Not specifying a path will default to stdout. Available formats: text, json.&lt;/code&gt; |
| Default |     &lt;code&gt;text&lt;/code&gt; |


