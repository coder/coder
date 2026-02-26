# Chats

## List chats

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats`

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "diff_status": {
      "additions": 0,
      "changed_files": 0,
      "changes_requested": true,
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "deletions": 0,
      "pull_request_state": "string",
      "refreshed_at": "2019-08-24T14:15:22Z",
      "stale_at": "2019-08-24T14:15:22Z",
      "url": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
    "status": "waiting",
    "title": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                            |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Chat](schemas.md#codersdkchat) |

<h3 id="list-chats-responseschema">Response Schema</h3>

Status Code **200**

| Name                     | Type                                                         | Required | Restrictions | Description |
|--------------------------|--------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`           | array                                                        | false    |              |             |
| `» created_at`           | string(date-time)                                            | false    |              |             |
| `» diff_status`          | [codersdk.ChatDiffStatus](schemas.md#codersdkchatdiffstatus) | false    |              |             |
| `»» additions`           | integer                                                      | false    |              |             |
| `»» changed_files`       | integer                                                      | false    |              |             |
| `»» changes_requested`   | boolean                                                      | false    |              |             |
| `»» chat_id`             | string(uuid)                                                 | false    |              |             |
| `»» deletions`           | integer                                                      | false    |              |             |
| `»» pull_request_state`  | string                                                       | false    |              |             |
| `»» refreshed_at`        | string(date-time)                                            | false    |              |             |
| `»» stale_at`            | string(date-time)                                            | false    |              |             |
| `»» url`                 | string                                                       | false    |              |             |
| `» id`                   | string(uuid)                                                 | false    |              |             |
| `» last_model_config_id` | string(uuid)                                                 | false    |              |             |
| `» owner_id`             | string(uuid)                                                 | false    |              |             |
| `» parent_chat_id`       | string(uuid)                                                 | false    |              |             |
| `» root_chat_id`         | string(uuid)                                                 | false    |              |             |
| `» status`               | [codersdk.ChatStatus](schemas.md#codersdkchatstatus)         | false    |              |             |
| `» title`                | string                                                       | false    |              |             |
| `» updated_at`           | string(date-time)                                            | false    |              |             |
| `» workspace_agent_id`   | string(uuid)                                                 | false    |              |             |
| `» workspace_id`         | string(uuid)                                                 | false    |              |             |

#### Enumerated Values

| Property | Value(s)                                                        |
|----------|-----------------------------------------------------------------|
| `status` | `completed`, `error`, `paused`, `pending`, `running`, `waiting` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats`

> Body parameter

```json
{
  "content": [
    {
      "text": "string",
      "type": "text"
    }
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description         |
|--------|------|--------------------------------------------------------------------|----------|---------------------|
| `body` | body | [codersdk.CreateChatRequest](schemas.md#codersdkcreatechatrequest) | true     | Create chat request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "deletions": 0,
    "pull_request_state": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat models

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/models \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/models`

### Example responses

> 200 Response

```json
{
  "providers": [
    {
      "available": true,
      "models": [
        {
          "display_name": "string",
          "id": "string",
          "model": "string",
          "provider": "string"
        }
      ],
      "provider": "string",
      "unavailable_reason": "missing_api_key"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatModelsResponse](schemas.md#codersdkchatmodelsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch chat list updates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/watch \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/watch`

### Example responses

> 200 Response

```json
{
  "data": null,
  "type": "ping"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ServerSentEvent](schemas.md#codersdkserversentevent) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get a chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "chat": {
    "created_at": "2019-08-24T14:15:22Z",
    "diff_status": {
      "additions": 0,
      "changed_files": 0,
      "changes_requested": true,
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "deletions": 0,
      "pull_request_state": "string",
      "refreshed_at": "2019-08-24T14:15:22Z",
      "stale_at": "2019-08-24T14:15:22Z",
      "url": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
    "status": "waiting",
    "title": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
  },
  "messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "data": [
            0
          ],
          "is_error": true,
          "media_type": "string",
          "result": [
            0
          ],
          "result_delta": "string",
          "signature": "string",
          "source_id": "string",
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
      "role": "string",
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
  ],
  "queued_messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        0
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatWithMessages](schemas.md#codersdkchatwithmessages) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete a chat

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/chats/{chat} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /chats/{chat}`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get diff contents for a chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/diff \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/diff`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "branch": "string",
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "diff": "string",
  "provider": "string",
  "pull_request_url": "string",
  "remote_origin": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDiffContents](schemas.md#codersdkchatdiffcontents) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get diff status for a chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/diff-status \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/diff-status`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "additions": 0,
  "changed_files": 0,
  "changes_requested": true,
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "deletions": 0,
  "pull_request_state": "string",
  "refreshed_at": "2019-08-24T14:15:22Z",
  "stale_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDiffStatus](schemas.md#codersdkchatdiffstatus) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Interrupt a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/interrupt \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats/{chat}/interrupt`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "deletions": 0,
    "pull_request_state": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a chat message

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
      "text": "string",
      "type": "text"
    }
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description                 |
|--------|------|----------------------------------------------------------------------------------|----------|-----------------------------|
| `chat` | path | string(uuid)                                                                     | true     | Chat ID                     |
| `body` | body | [codersdk.CreateChatMessageRequest](schemas.md#codersdkcreatechatmessagerequest) | true     | Create chat message request |

### Example responses

> 200 Response

```json
{
  "messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "data": [
            0
          ],
          "is_error": true,
          "media_type": "string",
          "result": [
            0
          ],
          "result_delta": "string",
          "signature": "string",
          "source_id": "string",
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
      "role": "string",
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
  ],
  "queued": true,
  "queued_message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      0
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                             |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.CreateChatMessageResponse](schemas.md#codersdkcreatechatmessageresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete a queued chat message

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/chats/{chat}/queue/{queuedMessage} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /chats/{chat}/queue/{queuedMessage}`

### Parameters

| Name            | In   | Type         | Required | Description       |
|-----------------|------|--------------|----------|-------------------|
| `chat`          | path | string(uuid) | true     | Chat ID           |
| `queuedMessage` | path | integer      | true     | Queued message ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Promote a queued message to send immediately

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/queue/{queuedMessage}/promote \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats/{chat}/queue/{queuedMessage}/promote`

### Parameters

| Name            | In   | Type         | Required | Description       |
|-----------------|------|--------------|----------|-------------------|
| `chat`          | path | string(uuid) | true     | Chat ID           |
| `queuedMessage` | path | integer      | true     | Queued message ID |

### Example responses

> 200 Response

```json
{
  "messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "data": [
            0
          ],
          "is_error": true,
          "media_type": "string",
          "result": [
            0
          ],
          "result_delta": "string",
          "signature": "string",
          "source_id": "string",
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
      "role": "string",
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
  ],
  "queued": true,
  "queued_message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      0
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                             |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.CreateChatMessageResponse](schemas.md#codersdkcreatechatmessageresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Stream chat updates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/stream \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/stream`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "data": null,
  "type": "ping"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ServerSentEvent](schemas.md#codersdkserversentevent) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
