<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder scaletest create-workspaces


Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.

It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.

## Usage
```console
coder scaletest create-workspaces [flags]
```


## Local Flags
| Name |  Default | Usage | Environment | 
| ---- |  ------- | ----- | -------- |
| --cleanup-concurrency |1 |<code>Number of concurrent cleanup jobs to run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_CLEANUP_CONCURRENCY</code>  |
| --cleanup-job-timeout |5m0s |<code>Timeout per job. Jobs may take longer to complete under higher concurrency limits.</code> | <code>$CODER_LOADTEST_CLEANUP_JOB_TIMEOUT</code>  |
| --cleanup-timeout |30m0s |<code>Timeout for the entire cleanup run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_CLEANUP_TIMEOUT</code>  |
| --concurrency |1 |<code>Number of concurrent jobs to run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_CONCURRENCY</code>  |
| --connect-hold |30s |<code>How long to hold the WireGuard connection open for.</code> | <code>$CODER_LOADTEST_CONNECT_HOLD</code>  |
| --connect-interval |1s |<code>How long to wait between making requests to the --connect-url once the connection is established.</code> | <code>$CODER_LOADTEST_CONNECT_INTERVAL</code>  |
| --connect-mode |derp |<code>Mode to use for connecting to the workspace. Can be 'derp' or 'direct'.</code> | <code>$CODER_LOADTEST_CONNECT_MODE</code>  |
| --connect-timeout |5s |<code>Timeout for each request to the --connect-url.</code> | <code>$CODER_LOADTEST_CONNECT_TIMEOUT</code>  |
| --connect-url | |<code>URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.</code> | <code>$CODER_LOADTEST_CONNECT_URL</code>  |
| --count, -c |1 |<code>Required: Number of workspaces to create.</code> | <code>$CODER_LOADTEST_COUNT</code>  |
| --job-timeout |5m0s |<code>Timeout per job. Jobs may take longer to complete under higher concurrency limits.</code> | <code>$CODER_LOADTEST_JOB_TIMEOUT</code>  |
| --no-cleanup |false |<code>Do not clean up resources after the test completes. You can cleanup manually using `coder scaletest cleanup`.</code> | <code>$CODER_LOADTEST_NO_CLEANUP</code>  |
| --no-plan |false |<code>Skip the dry-run step to plan the workspace creation. This step ensures that the given parameters are valid for the given template.</code> | <code>$CODER_LOADTEST_NO_PLAN</code>  |
| --no-wait-for-agents |false |<code>Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.</code> | <code>$CODER_LOADTEST_NO_WAIT_FOR_AGENTS</code>  |
| --output |[text] |<code>Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.</code> | <code>$CODER_SCALETEST_OUTPUTS</code>  |
| --parameter |[] |<code>Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value.</code> | <code>$CODER_LOADTEST_PARAMETERS</code>  |
| --parameters-file | |<code>Path to a YAML file containing the parameters to use for each workspace.</code> | <code>$CODER_LOADTEST_PARAMETERS_FILE</code>  |
| --run-command | |<code>Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.</code> | <code>$CODER_LOADTEST_RUN_COMMAND</code>  |
| --run-expect-output | |<code>Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.</code> | <code>$CODER_LOADTEST_RUN_EXPECT_OUTPUT</code>  |
| --run-expect-timeout |false |<code>Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.</code> | <code>$CODER_LOADTEST_RUN_EXPECT_TIMEOUT</code>  |
| --run-log-output |false |<code>Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.</code> | <code>$CODER_LOADTEST_RUN_LOG_OUTPUT</code>  |
| --run-timeout |5s |<code>Timeout for the command to complete.</code> | <code>$CODER_LOADTEST_RUN_TIMEOUT</code>  |
| --template, -t | |<code>Required: Name or ID of the template to use for workspaces.</code> | <code>$CODER_LOADTEST_TEMPLATE</code>  |
| --timeout |30m0s |<code>Timeout for the entire test run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_TIMEOUT</code>  |
| --trace |false |<code>Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.</code> | <code>$CODER_LOADTEST_TRACE</code>  |
| --trace-coder |false |<code>Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.</code> | <code>$CODER_LOADTEST_TRACE_CODER</code>  |
| --trace-honeycomb-api-key | |<code>Enables trace exporting to Honeycomb.io using the provided API key.</code> | <code>$CODER_LOADTEST_TRACE_HONEYCOMB_API_KEY</code>  |
| --trace-propagate |false |<code>Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.</code> | <code>$CODER_LOADTEST_TRACE_PROPAGATE</code>  |