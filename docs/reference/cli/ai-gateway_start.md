<!-- DO NOT EDIT | GENERATED CONTENT -->
# ai-gateway start

Run a standalone AI Gateway server

## Usage

```console
coder ai-gateway start [flags]
```

## Description

```console
Runs a standalone replica of the AI Gateway. Standalone replicas serve LLM client traffic on a dedicated HTTP listener and connect to coderd using the Coder deployment URL and an AI Gateway key.

Set --url or CODER_URL to the Coder deployment address, and set --key (CODER_AI_GATEWAY_KEY) or --key-file (CODER_AI_GATEWAY_KEY_FILE). A user login or session token is not required.
```

## Options

### --key

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>string</code>                |
| Environment | <code>$CODER_AI_GATEWAY_KEY</code> |

The AI Gateway key used to authenticate to coderd.

### --key-file

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_AI_GATEWAY_KEY_FILE</code> |

Path to a file containing the AI Gateway key used to authenticate to coderd.

### --http-address

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_AI_GATEWAY_HTTP_ADDRESS</code> |
| Default     | <code>127.0.0.1:4001</code>                 |

The bind address to serve incoming AI Gateway client traffic.

### --tls-cert-file

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_AI_GATEWAY_TLS_CERT_FILE</code> |

Path to a PEM-encoded TLS certificate. Enables TLS termination when set together with --tls-key-file.

### --tls-key-file

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_AI_GATEWAY_TLS_KEY_FILE</code> |

Path to a PEM-encoded TLS private key. Enables TLS termination when set together with --tls-cert-file.

### --prometheus-enable

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>bool</code>                            |
| Environment | <code>$CODER_PROMETHEUS_ENABLE</code>        |
| YAML        | <code>introspection.prometheus.enable</code> |

Serve prometheus metrics on the address defined by prometheus address.

### --prometheus-address

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>host:port</code>                        |
| Environment | <code>$CODER_PROMETHEUS_ADDRESS</code>        |
| YAML        | <code>introspection.prometheus.address</code> |
| Default     | <code>127.0.0.1:2112</code>                   |

The bind address to serve prometheus metrics.

### --trace

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_TRACE_ENABLE</code>          |
| YAML        | <code>introspection.tracing.enable</code> |

Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

### --trace-honeycomb-api-key

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_TRACE_HONEYCOMB_API_KEY</code> |

Enables trace exporting to Honeycomb.io using the provided API Key.

### --trace-logs

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_TRACE_LOGS</code>                 |
| YAML        | <code>introspection.tracing.captureLogs</code> |

Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs.

### -l, --log-filter

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string-array</code>                 |
| Environment | <code>$CODER_LOG_FILTER</code>            |
| YAML        | <code>introspection.logging.filter</code> |

Filter debug logs by matching against a given regex. Use .* to match all debug logs.

### --log-human

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_LOGGING_HUMAN</code>            |
| YAML        | <code>introspection.logging.humanPath</code> |
| Default     | <code>/dev/stderr</code>                     |

Output human-readable logs to a given file.

### --log-json

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_LOGGING_JSON</code>            |
| YAML        | <code>introspection.logging.jsonPath</code> |

Output JSON logs to a given file.

### --log-stackdriver

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>string</code>                                |
| Environment | <code>$CODER_LOGGING_STACKDRIVER</code>            |
| YAML        | <code>introspection.logging.stackdriverPath</code> |

Output Stackdriver compatible logs to a given file.

### --ai-gateway-max-concurrency

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>int</code>                               |
| Environment | <code>$CODER_AI_GATEWAY_MAX_CONCURRENCY</code> |
| YAML        | <code>ai_gateway.max_concurrency</code>        |
| Default     | <code>0</code>                                 |

Maximum number of concurrent AI Gateway requests per replica. Set to 0 to disable (unlimited).

### --ai-gateway-rate-limit

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>int</code>                          |
| Environment | <code>$CODER_AI_GATEWAY_RATE_LIMIT</code> |
| YAML        | <code>ai_gateway.rate_limit</code>        |
| Default     | <code>0</code>                            |

Maximum number of AI Gateway requests per second per replica. Set to 0 to disable (unlimited).

### --ai-gateway-send-actor-headers

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>bool</code>                                 |
| Environment | <code>$CODER_AI_GATEWAY_SEND_ACTOR_HEADERS</code> |
| YAML        | <code>ai_gateway.send_actor_headers</code>        |
| Default     | <code>false</code>                                |

Once enabled, extra headers will be added to upstream requests to identify the user (actor) making requests to AI Gateway. This is only needed if you are using a proxy between AI Gateway and an upstream AI provider. This will send X-Ai-Bridge-Actor-Id (the ID of the user making the request) and X-Ai-Bridge-Actor-Metadata-Username (their username).

### --ai-gateway-dump-dir

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_AI_GATEWAY_DUMP_DIR</code> |
| YAML        | <code>ai_gateway.api_dump_dir</code>    |

Base directory for dumping AI Gateway request/response pairs to disk for debugging. When set, each provider writes under a subdirectory named after the provider. Sensitive headers are redacted. Leave empty to disable.

### --ai-gateway-allow-byok

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_AI_GATEWAY_ALLOW_BYOK</code> |
| YAML        | <code>ai_gateway.allow_byok</code>        |
| Default     | <code>true</code>                         |

Allow users to provide their own LLM API keys or subscriptions. When disabled, only centralized key authentication is permitted.

### --ai-gateway-circuit-breaker-enabled

|             |                                                        |
|-------------|--------------------------------------------------------|
| Type        | <code>bool</code>                                      |
| Environment | <code>$CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED</code> |
| YAML        | <code>ai_gateway.circuit_breaker_enabled</code>        |
| Default     | <code>false</code>                                     |

Enable the circuit breaker to protect against cascading failures from upstream AI provider overload (503, 529).
