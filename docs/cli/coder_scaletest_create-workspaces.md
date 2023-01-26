## coder scaletest create-workspaces

Creates many workspaces and waits for them to be ready

### Synopsis

Creates many users, then creates a workspace for each user and waits for them finish building and fully come online. Optionally runs a command inside each workspace, and connects to the workspace over WireGuard.

It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.

```
coder scaletest create-workspaces [flags]
```

### Options

```
      --cleanup-concurrency int              Number of concurrent cleanup jobs to run. 0 means unlimited.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CLEANUP_CONCURRENCY[0m (default 1)
      --cleanup-job-timeout duration         Timeout per job. Jobs may take longer to complete under higher concurrency limits.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CLEANUP_JOB_TIMEOUT[0m (default 5m0s)
      --cleanup-timeout duration             Timeout for the entire cleanup run. 0 means unlimited.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CLEANUP_TIMEOUT[0m (default 30m0s)
      --concurrency int                      Number of concurrent jobs to run. 0 means unlimited.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONCURRENCY[0m (default 1)
      --connect-hold duration                How long to hold the WireGuard connection open for.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONNECT_HOLD[0m (default 30s)
      --connect-interval duration            How long to wait between making requests to the --connect-url once the connection is established.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONNECT_INTERVAL[0m (default 1s)
      --connect-mode string                  Mode to use for connecting to the workspace. Can be 'derp' or 'direct'.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONNECT_MODE[0m (default "derp")
      --connect-timeout duration             Timeout for each request to the --connect-url.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONNECT_TIMEOUT[0m (default 5s)
      --connect-url string                   URL to connect to inside the the workspace over WireGuard. If not specified, no connections will be made over WireGuard.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_CONNECT_URL[0m
  -c, --count int                            Required: Number of workspaces to create.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_COUNT[0m (default 1)
  -h, --help                                 help for create-workspaces
      --job-timeout duration                 Timeout per job. Jobs may take longer to complete under higher concurrency limits.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_JOB_TIMEOUT[0m (default 5m0s)
      --no-cleanup coder scaletest cleanup   Do not clean up resources after the test completes. You can cleanup manually using coder scaletest cleanup.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_NO_CLEANUP[0m
      --no-plan                              Skip the dry-run step to plan the workspace creation. This step ensures that the given parameters are valid for the given template.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_NO_PLAN[0m
      --no-wait-for-agents                   Do not wait for agents to start before marking the test as succeeded. This can be useful if you are running the test against a template that does not start the agent quickly.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_NO_WAIT_FOR_AGENTS[0m
      --output stringArray                   Output format specs in the format "<format>[:<path>]". Not specifying a path will default to stdout. Available formats: text, json.
                                             [38;2;88;88;88mConsumes $CODER_SCALETEST_OUTPUTS[0m (default [text])
      --parameter stringArray                Parameters to use for each workspace. Can be specified multiple times. Overrides any existing parameters with the same name from --parameters-file. Format: key=value.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_PARAMETERS[0m
      --parameters-file string               Path to a YAML file containing the parameters to use for each workspace.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_PARAMETERS_FILE[0m
      --run-command string                   Command to run inside each workspace using reconnecting-pty (i.e. web terminal protocol). If not specified, no command will be run.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_RUN_COMMAND[0m
      --run-expect-output string             Expect the command to output the given string (on a single line). If the command does not output the given string, it will be marked as failed.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_RUN_EXPECT_OUTPUT[0m
      --run-expect-timeout                   Expect the command to timeout. If the command does not finish within the given --run-timeout, it will be marked as succeeded. If the command finishes before the timeout, it will be marked as failed.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_RUN_EXPECT_TIMEOUT[0m
      --run-log-output                       Log the output of the command to the test logs. This should be left off unless you expect small amounts of output. Large amounts of output will cause high memory usage.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_RUN_LOG_OUTPUT[0m
      --run-timeout duration                 Timeout for the command to complete.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_RUN_TIMEOUT[0m (default 5s)
  -t, --template string                      Required: Name or ID of the template to use for workspaces.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TEMPLATE[0m
      --timeout duration                     Timeout for the entire test run. 0 means unlimited.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TIMEOUT[0m (default 30m0s)
      --trace                                Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TRACE[0m
      --trace-coder                          Whether opentelemetry traces are sent to Coder. We recommend keeping this disabled unless we advise you to enable it.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TRACE_CODER[0m
      --trace-honeycomb-api-key string       Enables trace exporting to Honeycomb.io using the provided API key.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TRACE_HONEYCOMB_API_KEY[0m
      --trace-propagate                      Enables trace propagation to the Coder backend, which will be used to correlate server-side spans with client-side spans. Only enable this if the server is configured with the exact same tracing configuration as the client.
                                             [38;2;88;88;88mConsumes $CODER_LOADTEST_TRACE_PROPAGATE[0m
```

### Options inherited from parent commands

```
      --global-config coder   Path to the global coder config directory.
                              [38;2;88;88;88mConsumes $CODER_CONFIG_DIR[0m (default "/home/coder/.config/coderv2")
      --header stringArray    HTTP headers added to all requests. Provide as "Key=Value".
                              [38;2;88;88;88mConsumes $CODER_HEADER[0m
      --no-feature-warning    Suppress warnings about unlicensed features.
                              [38;2;88;88;88mConsumes $CODER_NO_FEATURE_WARNING[0m
      --no-version-warning    Suppress warning when client and server versions do not match.
                              [38;2;88;88;88mConsumes $CODER_NO_VERSION_WARNING[0m
      --token string          Specify an authentication token. For security reasons setting CODER_SESSION_TOKEN is preferred.
                              [38;2;88;88;88mConsumes $CODER_SESSION_TOKEN[0m
      --url string            URL to a deployment.
                              [38;2;88;88;88mConsumes $CODER_URL[0m
  -v, --verbose               Enable verbose output.
                              [38;2;88;88;88mConsumes $CODER_VERBOSE[0m
```

### SEE ALSO

- [coder scaletest](coder_scaletest.md) - Run a scale test against the Coder API

###### Auto generated by spf13/cobra on 26-Jan-2023
