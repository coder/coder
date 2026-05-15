# AI Bridge

## List AI Bridge providers

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai/providers \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai/providers`

### Example responses

> 200 Response

```json
[
  {
    "base_url": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "enabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "settings": {},
    "type": "openai",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIProvider](schemas.md#codersdkaiprovider) |

<h3 id="list-ai-bridge-providers-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type                                                                 | Required | Restrictions | Description |
|------------------|----------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`   | array                                                                | false    |              |             |
| `» base_url`     | string                                                               | false    |              |             |
| `» created_at`   | string(date-time)                                                    | false    |              |             |
| `» display_name` | string                                                               | false    |              |             |
| `» enabled`      | boolean                                                              | false    |              |             |
| `» id`           | string(uuid)                                                         | false    |              |             |
| `» name`         | string                                                               | false    |              |             |
| `» settings`     | [codersdk.AIProviderSettings](schemas.md#codersdkaiprovidersettings) | false    |              |             |
| `» type`         | [codersdk.AIProviderType](schemas.md#codersdkaiprovidertype)         | false    |              |             |
| `» updated_at`   | string(date-time)                                                    | false    |              |             |

#### Enumerated Values

| Property | Value(s)                                                                    |
|----------|-----------------------------------------------------------------------------|
| `type`   | `anthropic`, `azure`, `bedrock`, `google`, `openai`, `openrouter`, `vercel` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI Bridge provider

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/ai/providers \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/ai/providers`

> Body parameter

```json
{
  "base_url": "string",
  "display_name": "string",
  "enabled": true,
  "name": "string",
  "settings": {},
  "type": "openai"
}
```

### Parameters

| Name   | In   | Type                                                                           | Required | Description                |
|--------|------|--------------------------------------------------------------------------------|----------|----------------------------|
| `body` | body | [codersdk.CreateAIProviderRequest](schemas.md#codersdkcreateaiproviderrequest) | true     | Create AI provider request |

### Example responses

> 201 Response

```json
{
  "base_url": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "settings": {},
  "type": "openai",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                               |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIProvider](schemas.md#codersdkaiprovider) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get an AI Bridge provider

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai/providers/{idOrName} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai/providers/{idOrName}`

### Parameters

| Name       | In   | Type   | Required | Description         |
|------------|------|--------|----------|---------------------|
| `idOrName` | path | string | true     | Provider ID or name |

### Example responses

> 200 Response

```json
{
  "base_url": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "settings": {},
  "type": "openai",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIProvider](schemas.md#codersdkaiprovider) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an AI Bridge provider

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/ai/providers/{idOrName} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/ai/providers/{idOrName}`

### Parameters

| Name       | In   | Type   | Required | Description         |
|------------|------|--------|----------|---------------------|
| `idOrName` | path | string | true     | Provider ID or name |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update an AI Bridge provider

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/ai/providers/{idOrName} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/ai/providers/{idOrName}`

> Body parameter

```json
{
  "base_url": "string",
  "display_name": "string",
  "enabled": true,
  "settings": {}
}
```

### Parameters

| Name       | In   | Type                                                                           | Required | Description                |
|------------|------|--------------------------------------------------------------------------------|----------|----------------------------|
| `idOrName` | path | string                                                                         | true     | Provider ID or name        |
| `body`     | body | [codersdk.UpdateAIProviderRequest](schemas.md#codersdkupdateaiproviderrequest) | true     | Update AI provider request |

### Example responses

> 200 Response

```json
{
  "base_url": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "settings": {},
  "type": "openai",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIProvider](schemas.md#codersdkaiprovider) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Bridge provider keys

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai/providers/{idOrName}/keys \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai/providers/{idOrName}/keys`

### Parameters

| Name       | In   | Type   | Required | Description         |
|------------|------|--------|----------|---------------------|
| `idOrName` | path | string | true     | Provider ID or name |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                              |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIProviderKey](schemas.md#codersdkaiproviderkey) |

<h3 id="list-ai-bridge-provider-keys-responseschema">Response Schema</h3>

Status Code **200**

| Name            | Type              | Required | Restrictions | Description |
|-----------------|-------------------|----------|--------------|-------------|
| `[array item]`  | array             | false    |              |             |
| `» created_at`  | string(date-time) | false    |              |             |
| `» id`          | string(uuid)      | false    |              |             |
| `» provider_id` | string(uuid)      | false    |              |             |
| `» updated_at`  | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI Bridge provider key

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/ai/providers/{idOrName}/keys \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/ai/providers/{idOrName}/keys`

> Body parameter

```json
{
  "api_key": "string"
}
```

### Parameters

| Name       | In   | Type                                                                                 | Required | Description                    |
|------------|------|--------------------------------------------------------------------------------------|----------|--------------------------------|
| `idOrName` | path | string                                                                               | true     | Provider ID or name            |
| `body`     | body | [codersdk.CreateAIProviderKeyRequest](schemas.md#codersdkcreateaiproviderkeyrequest) | true     | Create AI provider key request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                     |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIProviderKey](schemas.md#codersdkaiproviderkey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an AI Bridge provider key

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/ai/providers/{idOrName}/keys/{keyID} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/ai/providers/{idOrName}/keys/{keyID}`

### Parameters

| Name       | In   | Type         | Required | Description         |
|------------|------|--------------|----------|---------------------|
| `idOrName` | path | string       | true     | Provider ID or name |
| `keyID`    | path | string(uuid) | true     | Key ID              |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Bridge clients

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/clients \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/clients`

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

<h3 id="list-ai-bridge-clients-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Bridge interceptions

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/interceptions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/interceptions`

### Parameters

| Name       | In    | Type    | Required | Description                                                                                                            |
|------------|-------|---------|----------|------------------------------------------------------------------------------------------------------------------------|
| `q`        | query | string  | false    | Search query in the format `key:value`. Available keys are: initiator, provider, model, started_after, started_before. |
| `limit`    | query | integer | false    | Page limit                                                                                                             |
| `after_id` | query | string  | false    | Cursor pagination after ID (cannot be used with offset)                                                                |
| `offset`   | query | integer | false    | Offset pagination (cannot be used with after_id)                                                                       |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "results": [
    {
      "api_key_id": "string",
      "client": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
      "model": "string",
      "provider": "string",
      "provider_name": "string",
      "started_at": "2019-08-24T14:15:22Z",
      "token_usages": [
        {
          "cache_read_input_tokens": 0,
          "cache_write_input_tokens": 0,
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "input_tokens": 0,
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "output_tokens": 0,
          "provider_response_id": "string"
        }
      ],
      "tool_usages": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "injected": true,
          "input": "string",
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "invocation_error": "string",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "provider_response_id": "string",
          "server_url": "string",
          "tool": "string"
        }
      ],
      "user_prompts": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "prompt": "string",
          "provider_response_id": "string"
        }
      ]
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIBridgeListInterceptionsResponse](schemas.md#codersdkaibridgelistinterceptionsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Bridge models

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/models \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/models`

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

<h3 id="list-ai-bridge-models-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI Bridge sessions

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/sessions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/sessions`

### Parameters

| Name               | In    | Type    | Required | Description                                                                                                                                |
|--------------------|-------|---------|----------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `q`                | query | string  | false    | Search query in the format `key:value`. Available keys are: initiator, provider, model, client, session_id, started_after, started_before. |
| `limit`            | query | integer | false    | Page limit                                                                                                                                 |
| `after_session_id` | query | string  | false    | Cursor pagination after session ID (cannot be used with offset)                                                                            |
| `offset`           | query | integer | false    | Offset pagination (cannot be used with after_session_id)                                                                                   |

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

## Get AI Bridge session threads

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/sessions/{session_id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/sessions/{session_id}`

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
