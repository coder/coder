# AI Gateway

## List AI model prices

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/ai/model-prices \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/ai/model-prices`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "cache_read_price": 0,
    "cache_write_price": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "input_price": 0,
    "model": "string",
    "output_price": 0,
    "provider": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIModelPrice](schemas.md#codersdkaimodelprice) |

<h3 id="list-ai-model-prices-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type              | Required | Restrictions | Description |
|-----------------------|-------------------|----------|--------------|-------------|
| `[array item]`        | array             | false    |              |             |
| `» cache_read_price`  | integer           | false    |              |             |
| `» cache_write_price` | integer           | false    |              |             |
| `» created_at`        | string(date-time) | false    |              |             |
| `» input_price`       | integer           | false    |              |             |
| `» model`             | string            | false    |              |             |
| `» output_price`      | integer           | false    |              |             |
| `» provider`          | string            | false    |              |             |
| `» updated_at`        | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert AI model prices

### Code samples

```sh
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/ai/model-prices \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/ai/model-prices`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "cache_read_price": 0,
  "cache_write_price": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "input_price": 0,
  "model": "string",
  "output_price": 0,
  "provider": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Parameters

| Name   | In   | Type                                                     | Required | Description  |
|--------|------|----------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.AIModelPrice](schemas.md#codersdkaimodelprice) | true     | Model prices |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Gateway clients

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/clients \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/clients`

Alias: also available at /api/v2/aibridge/clients for backward compatibility.

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="list-ai-gateway-clients-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Gateway models

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/models \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/models`

Alias: also available at /api/v2/aibridge/models for backward compatibility.

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="list-ai-gateway-models-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Gateway sessions

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/sessions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/sessions`

Alias: also available at /api/v2/aibridge/sessions for backward compatibility.

### Parameters

| Name               | In    | Type    | Required | Description                                                                                                                                               |
|--------------------|-------|---------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| `q`                | query | string  | false    | Search query in the format `key:value`. Available keys are: initiator, provider, provider_name, model, client, session_id, started_after, started_before. |
| `limit`            | query | integer | false    | Page limit                                                                                                                                                |
| `after_session_id` | query | string  | false    | Cursor pagination after session ID (cannot be used with offset)                                                                                           |
| `offset`           | query | integer | false    | Offset pagination (cannot be used with after_session_id)                                                                                                  |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "sessions": [
    {
      "client": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "id": "string",
      "initiator": {
        "avatar_url": "http://example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "name": "string",
        "username": "string"
      },
      "last_active_at": "2019-08-24T14:15:22Z",
      "last_prompt": "string",
      "metadata": {
        "property1": null,
        "property2": null
      },
      "models": [
        "string"
      ],
      "network_calls": {
        "blocked": 0,
        "total": 0
      },
      "providers": [
        "string"
      ],
      "started_at": "2019-08-24T14:15:22Z",
      "threads": 0,
      "token_usage_summary": {
        "cache_read_input_tokens": 0,
        "cache_write_input_tokens": 0,
        "input_tokens": 0,
        "output_tokens": 0
      }
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIBridgeListSessionsResponse](schemas.md#codersdkaibridgelistsessionsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get AI Gateway session threads

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/sessions/{session_id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/sessions/{session_id}`

Alias: also available at /api/v2/aibridge/sessions/{session_id} for backward compatibility.

### Parameters

| Name         | In    | Type    | Required | Description                                         |
|--------------|-------|---------|----------|-----------------------------------------------------|
| `session_id` | path  | string  | true     | Session ID (client_session_id or interception UUID) |
| `after_id`   | query | string  | false    | Thread pagination cursor (forward/older)            |
| `before_id`  | query | string  | false    | Thread pagination cursor (backward/newer)           |
| `limit`      | query | integer | false    | Number of threads per page (default 50)             |

### Example responses

> 200 Response

```json
{
  "client": "string",
  "ended_at": "2019-08-24T14:15:22Z",
  "id": "string",
  "initiator": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "username": "string"
  },
  "metadata": {
    "property1": null,
    "property2": null
  },
  "models": [
    "string"
  ],
  "page_ended_at": "2019-08-24T14:15:22Z",
  "page_started_at": "2019-08-24T14:15:22Z",
  "providers": [
    "string"
  ],
  "started_at": "2019-08-24T14:15:22Z",
  "threads": [
    {
      "agent_firewall_sequence_number": 0,
      "agent_firewall_session_id": "3735294f-18b1-4e7a-a269-99c30f0b30e7",
      "agentic_actions": [
        {
          "model": "string",
          "thinking": [
            {
              "text": "string"
            }
          ],
          "token_usage": {
            "cache_read_input_tokens": 0,
            "cache_write_input_tokens": 0,
            "input_tokens": 0,
            "metadata": {
              "property1": null,
              "property2": null
            },
            "output_tokens": 0
          },
          "tool_calls": [
            {
              "created_at": "2019-08-24T14:15:22Z",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "injected": true,
              "input": "string",
              "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
              "metadata": {
                "property1": null,
                "property2": null
              },
              "provider_response_id": "string",
              "server_url": "string",
              "tool": "string"
            }
          ]
        }
      ],
      "credential_hint": "string",
      "credential_kind": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "error_message": "string",
      "error_type": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "model": "string",
      "prompt": "string",
      "provider": "string",
      "started_at": "2019-08-24T14:15:22Z",
      "token_usage": {
        "cache_read_input_tokens": 0,
        "cache_write_input_tokens": 0,
        "input_tokens": 0,
        "metadata": {
          "property1": null,
          "property2": null
        },
        "output_tokens": 0
      }
    }
  ],
  "token_usage_summary": {
    "cache_read_input_tokens": 0,
    "cache_write_input_tokens": 0,
    "input_tokens": 0,
    "metadata": {
      "property1": null,
      "property2": null
    },
    "output_tokens": 0
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIBridgeSessionThreadsResponse](schemas.md#codersdkaibridgesessionthreadsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
