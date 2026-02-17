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
    "model_config": [
      0
    ],
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
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

| Name                    | Type                                                         | Required | Restrictions | Description |
|-------------------------|--------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`          | array                                                        | false    |              |             |
| `» created_at`          | string(date-time)                                            | false    |              |             |
| `» diff_status`         | [codersdk.ChatDiffStatus](schemas.md#codersdkchatdiffstatus) | false    |              |             |
| `»» additions`          | integer                                                      | false    |              |             |
| `»» changed_files`      | integer                                                      | false    |              |             |
| `»» changes_requested`  | boolean                                                      | false    |              |             |
| `»» chat_id`            | string(uuid)                                                 | false    |              |             |
| `»» deletions`          | integer                                                      | false    |              |             |
| `»» pull_request_state` | string                                                       | false    |              |             |
| `»» refreshed_at`       | string(date-time)                                            | false    |              |             |
| `»» stale_at`           | string(date-time)                                            | false    |              |             |
| `»» url`                | string                                                       | false    |              |             |
| `» id`                  | string(uuid)                                                 | false    |              |             |
| `» model_config`        | array                                                        | false    |              |             |
| `» owner_id`            | string(uuid)                                                 | false    |              |             |
| `» status`              | [codersdk.ChatStatus](schemas.md#codersdkchatstatus)         | false    |              |             |
| `» title`               | string                                                       | false    |              |             |
| `» updated_at`          | string(date-time)                                            | false    |              |             |
| `» workspace_agent_id`  | string(uuid)                                                 | false    |              |             |
| `» workspace_id`        | string(uuid)                                                 | false    |              |             |

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
  "input": {
    "parts": [
      {
        "text": "string",
        "type": "text"
      }
    ]
  },
  "message": "string",
  "model": "string",
  "model_config": [
    0
  ],
  "system_prompt": "string",
  "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1",
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
  "model_config": [
    0
  ],
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
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
    "model_config": [
      0
    ],
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
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
        0
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "hidden": true,
      "id": 0,
      "parts": [
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
          "result_meta": {
            "content": "string",
            "created": true,
            "error": "string",
            "exit_code": 0,
            "mime_type": "string",
            "output": "string",
            "reason": "string",
            "workspace_agent_id": "string",
            "workspace_id": "string",
            "workspace_name": "string",
            "workspace_url": "string"
          },
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
      "role": "string",
      "thinking": "string",
      "tool_call_id": "string"
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
  "model_config": [
    0
  ],
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
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
    0
  ],
  "role": "string",
  "thinking": "string",
  "tool_call_id": "string"
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
[
  {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      0
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "hidden": true,
    "id": 0,
    "parts": [
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
        "result_meta": {
          "content": "string",
          "created": true,
          "error": "string",
          "exit_code": 0,
          "mime_type": "string",
          "output": "string",
          "reason": "string",
          "workspace_agent_id": "string",
          "workspace_id": "string",
          "workspace_name": "string",
          "workspace_url": "string"
        },
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
    "role": "string",
    "thinking": "string",
    "tool_call_id": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                          |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatMessage](schemas.md#codersdkchatmessage) |

<h3 id="create-a-chat-message-responseschema">Response Schema</h3>

Status Code **200**

| Name                     | Type                                                                         | Required | Restrictions | Description |
|--------------------------|------------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`           | array                                                                        | false    |              |             |
| `» chat_id`              | string(uuid)                                                                 | false    |              |             |
| `» content`              | array                                                                        | false    |              |             |
| `» created_at`           | string(date-time)                                                            | false    |              |             |
| `» hidden`               | boolean                                                                      | false    |              |             |
| `» id`                   | integer                                                                      | false    |              |             |
| `» parts`                | array                                                                        | false    |              |             |
| `»» args`                | array                                                                        | false    |              |             |
| `»» args_delta`          | string                                                                       | false    |              |             |
| `»» data`                | array                                                                        | false    |              |             |
| `»» is_error`            | boolean                                                                      | false    |              |             |
| `»» media_type`          | string                                                                       | false    |              |             |
| `»» result`              | array                                                                        | false    |              |             |
| `»» result_delta`        | string                                                                       | false    |              |             |
| `»» result_meta`         | [codersdk.ChatToolResultMetadata](schemas.md#codersdkchattoolresultmetadata) | false    |              |             |
| `»»» content`            | string                                                                       | false    |              |             |
| `»»» created`            | boolean                                                                      | false    |              |             |
| `»»» error`              | string                                                                       | false    |              |             |
| `»»» exit_code`          | integer                                                                      | false    |              |             |
| `»»» mime_type`          | string                                                                       | false    |              |             |
| `»»» output`             | string                                                                       | false    |              |             |
| `»»» reason`             | string                                                                       | false    |              |             |
| `»»» workspace_agent_id` | string                                                                       | false    |              |             |
| `»»» workspace_id`       | string                                                                       | false    |              |             |
| `»»» workspace_name`     | string                                                                       | false    |              |             |
| `»»» workspace_url`      | string                                                                       | false    |              |             |
| `»» signature`           | string                                                                       | false    |              |             |
| `»» source_id`           | string                                                                       | false    |              |             |
| `»» text`                | string                                                                       | false    |              |             |
| `»» title`               | string                                                                       | false    |              |             |
| `»» tool_call_id`        | string                                                                       | false    |              |             |
| `»» tool_name`           | string                                                                       | false    |              |             |
| `»» type`                | [codersdk.ChatMessagePartType](schemas.md#codersdkchatmessageparttype)       | false    |              |             |
| `»» url`                 | string                                                                       | false    |              |             |
| `» role`                 | string                                                                       | false    |              |             |
| `» thinking`             | string                                                                       | false    |              |             |
| `» tool_call_id`         | string                                                                       | false    |              |             |

#### Enumerated Values

| Property | Value(s)                                                          |
|----------|-------------------------------------------------------------------|
| `type`   | `file`, `reasoning`, `source`, `text`, `tool-call`, `tool-result` |

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
