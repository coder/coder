# Chat

## Get chat usage limit config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/usage-limits \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/usage-limits`

### Example responses

> 200 Response

```json
{
  "overrides": [
    {
      "avatar_url": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "name": "string",
      "spend_limit_micros": 0,
      "updated_at": "2019-08-24T14:15:22Z",
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
      "username": "string"
    }
  ],
  "period": "day",
  "spend_limit_micros": 0,
  "unpriced_model_count": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatUsageLimitConfigResponse](schemas.md#codersdkchatusagelimitconfigresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat usage limit config

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/chats/usage-limits \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /chats/usage-limits`

> Body parameter

```json
{
  "period": "day",
  "spend_limit_micros": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Parameters

| Name   | In   | Type                                                                     | Required | Description |
|--------|------|--------------------------------------------------------------------------|----------|-------------|
| `body` | body | [codersdk.ChatUsageLimitConfig](schemas.md#codersdkchatusagelimitconfig) | true     | Config      |

### Example responses

> 200 Response

```json
{
  "period": "day",
  "spend_limit_micros": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatUsageLimitConfig](schemas.md#codersdkchatusagelimitconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert chat usage limit override

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/chats/usage-limits/overrides/{user} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /chats/usage-limits/overrides/{user}`

> Body parameter

```json
{
  "spend_limit_micros": 0
}
```

### Parameters

| Name   | In   | Type                                                                                                   | Required | Description |
|--------|------|--------------------------------------------------------------------------------------------------------|----------|-------------|
| `user` | path | string(uuid)                                                                                           | true     | User ID     |
| `body` | body | [codersdk.UpsertChatUsageLimitOverrideRequest](schemas.md#codersdkupsertchatusagelimitoverriderequest) | true     | Override    |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "name": "string",
  "spend_limit_micros": 0,
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatUsageLimitOverride](schemas.md#codersdkchatusagelimitoverride) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete chat usage limit override

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/chats/usage-limits/overrides/{user} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /chats/usage-limits/overrides/{user}`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `user` | path | string(uuid) | true     | User ID     |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get my chat usage limit status

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/usage-limits/status \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/usage-limits/status`

### Example responses

> 200 Response

```json
{
  "current_spend": 0,
  "is_limited": true,
  "period": "day",
  "period_end": "2019-08-24T14:15:22Z",
  "period_start": "2019-08-24T14:15:22Z",
  "spend_limit_micros": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatUsageLimitStatus](schemas.md#codersdkchatusagelimitstatus) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send a chat message

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/messages \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats/{chat}/messages`

> Body parameter

```json
{
  "content": [
    {
      "content": "string",
      "end_line": 0,
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "file_name": "string",
      "start_line": 0,
      "text": "string",
      "type": "text"
    }
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description |
|--------|------|----------------------------------------------------------------------------------|----------|-------------|
| `chat` | path | string(uuid)                                                                     | true     | Chat ID     |
| `body` | body | [codersdk.CreateChatMessageRequest](schemas.md#codersdkcreatechatmessagerequest) | true     | Message     |

### Example responses

> 200 Response

```json
{
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "content": [
    {
      "args": [
        0
      ],
      "args_delta": "string",
      "content": "string",
      "data": [
        0
      ],
      "end_line": 0,
      "file_id": {
        "uuid": "string",
        "valid": true
      },
      "file_name": "string",
      "is_error": true,
      "media_type": "string",
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
      "signature": "string",
      "source_id": "string",
      "start_line": 0,
      "text": "string",
      "title": "string",
      "tool_call_id": "string",
      "tool_name": "string",
      "type": "text",
      "url": "string"
    }
  ],
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
  "id": 0,
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "role": "system",
  "usage": {
    "cache_creation_tokens": 0,
    "cache_read_tokens": 0,
    "context_limit": 0,
    "input_tokens": 0,
    "output_tokens": 0,
    "reasoning_tokens": 0,
    "total_tokens": 0
  }
}
```

### Responses

| Status | Meaning                                                       | Description          | Schema                                                 |
|--------|---------------------------------------------------------------|----------------------|--------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)       | OK                   | [codersdk.ChatMessage](schemas.md#codersdkchatmessage) |
| 409    | [Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8) | Usage limit exceeded | [codersdk.Response](schemas.md#codersdkresponse)       |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
