<!-- DO NOT EDIT | GENERATED CONTENT -->
# ai-gateway start

Run a standalone AI Gateway server

## Usage

```console
coder ai-gateway start [flags]
```

## Description

```console
The standalone AI Gateway connects to a Coder deployment over DRPC to authenticate users, record interceptions, and configure MCP, while serving LLM client traffic on its own HTTP listener.

The deployment address is taken from the global --url flag (CODER_URL) and is required. The gateway authenticates with the key from --key (CODER_AI_GATEWAY_KEY). Provider and other AI Gateway settings use the same CODER_AI_GATEWAY_* options as embedded mode.
```

## Options

### --key

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>string</code>                |
| Environment | <code>$CODER_AI_GATEWAY_KEY</code> |

The AI Gateway key used to authenticate to coderd.

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

### --ai-gateway-enabled

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>bool</code>                      |
| Environment | <code>$CODER_AI_GATEWAY_ENABLED</code> |
| YAML        | <code>ai_gateway.enabled</code>        |
| Default     | <code>true</code>                      |

Whether to start an in-memory AI Gateway instance.

### --ai-gateway-openai-base-url

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>string</code>                            |
| Environment | <code>$CODER_AI_GATEWAY_OPENAI_BASE_URL</code> |
| YAML        | <code>ai_gateway.openai_base_url</code>        |
| Default     | <code>https://api.openai.com/v1/</code>        |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL of the OpenAI API.

### --ai-gateway-openai-key

|             |                                           |
|-------------|-------------------------------------------|
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_AI_GATEWAY_OPENAI_KEY</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The key to authenticate against the OpenAI API.

### --ai-gateway-anthropic-base-url

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>string</code>                               |
| Environment | <code>$CODER_AI_GATEWAY_ANTHROPIC_BASE_URL</code> |
| YAML        | <code>ai_gateway.anthropic_base_url</code>        |
| Default     | <code>https://api.anthropic.com/</code>           |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL of the Anthropic API.

### --ai-gateway-anthropic-key

|             |                                              |
|-------------|----------------------------------------------|
| Type        | <code>string</code>                          |
| Environment | <code>$CODER_AI_GATEWAY_ANTHROPIC_KEY</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The key to authenticate against the Anthropic API.

### --ai-gateway-bedrock-base-url

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_BASE_URL</code> |
| YAML        | <code>ai_gateway.bedrock_base_url</code>        |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The base URL to use for the AWS Bedrock API. Use this setting to specify an exact URL to use. Takes precedence over CODER_AI_GATEWAY_BEDROCK_REGION.

### --ai-gateway-bedrock-region

|             |                                               |
|-------------|-----------------------------------------------|
| Type        | <code>string</code>                           |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_REGION</code> |
| YAML        | <code>ai_gateway.bedrock_region</code>        |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The AWS Bedrock API region to use. Constructs a base URL to use for the AWS Bedrock API in the form of 'https://bedrock-runtime.<region>.amazonaws.com'.

### --ai-gateway-bedrock-access-key

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>string</code>                               |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The access key to authenticate against the AWS Bedrock API.

### --ai-gateway-bedrock-access-key-secret

|             |                                                          |
|-------------|----------------------------------------------------------|
| Type        | <code>string</code>                                      |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY_SECRET</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The access key secret to use with the access key to authenticate against the AWS Bedrock API.

### --ai-gateway-bedrock-model

|             |                                                               |
|-------------|---------------------------------------------------------------|
| Type        | <code>string</code>                                           |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_MODEL</code>                  |
| YAML        | <code>ai_gateway.bedrock_model</code>                         |
| Default     | <code>global.anthropic.claude-sonnet-4-5-20250929-v1:0</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The model to use when making requests to the AWS Bedrock API.

### --ai-gateway-bedrock-small-fastmodel

|             |                                                              |
|-------------|--------------------------------------------------------------|
| Type        | <code>string</code>                                          |
| Environment | <code>$CODER_AI_GATEWAY_BEDROCK_SMALL_FAST_MODEL</code>      |
| YAML        | <code>ai_gateway.bedrock_small_fast_model</code>             |
| Default     | <code>global.anthropic.claude-haiku-4-5-20251001-v1:0</code> |

Deprecated: manage AI Providers from the Coder UI or HTTP API. If set, this option seeds provider configuration at startup only exactly once. It will not be used in service runtime. The small fast model to use when making requests to the AWS Bedrock API. Claude Code uses Haiku-class models to perform background tasks. See https://docs.claude.com/en/docs/claude-code/settings#environment-variables.

### --ai-gateway-retention

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>duration</code>                    |
| Environment | <code>$CODER_AI_GATEWAY_RETENTION</code> |
| YAML        | <code>ai_gateway.retention</code>        |
| Default     | <code>60d</code>                         |

Length of time to retain data such as interceptions and all related records (token, prompt, tool use).

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

### --ai-gateway-structured-logging

|             |                                                   |
|-------------|---------------------------------------------------|
| Type        | <code>bool</code>                                 |
| Environment | <code>$CODER_AI_GATEWAY_STRUCTURED_LOGGING</code> |
| YAML        | <code>ai_gateway.structured_logging</code>        |
| Default     | <code>false</code>                                |

Emit structured logs for AI Gateway interception records. Use this for exporting these records to external SIEM or observability systems.

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

Base directory for dumping AI Bridge request/response pairs to disk for debugging. When set, each provider writes under a subdirectory named after the provider. Sensitive headers are redacted. Leave empty to disable.

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

### --ai-budget-policy

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>highest</code>                  |
| Environment | <code>$CODER_AI_BUDGET_POLICY</code>  |
| YAML        | <code>ai_gateway.budget_policy</code> |
| Default     | <code>highest</code>                  |

Determines the effective group when a user belongs to multiple groups with AI budgets. "highest" selects the group with the largest spend limit, and is currently the only supported value.

### --ai-budget-period

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>month</code>                    |
| Environment | <code>$CODER_AI_BUDGET_PERIOD</code>  |
| YAML        | <code>ai_gateway.budget_period</code> |
| Default     | <code>month</code>                    |

Determines when accumulated AI spend resets to zero, aligned to UTC calendar boundaries. Only "month" is currently supported.
