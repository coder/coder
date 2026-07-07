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

### --verbose

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_AI_GATEWAY_VERBOSE</code> |
| Default     | <code>false</code>                     |

Output debug-level logs.

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
