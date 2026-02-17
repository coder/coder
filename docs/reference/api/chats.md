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

| Name                   | Type                                                 | Required | Restrictions | Description |
|------------------------|------------------------------------------------------|----------|--------------|-------------|
| `[array item]`         | array                                                | false    |              |             |
| `» created_at`         | string(date-time)                                    | false    |              |             |
| `» id`                 | string(uuid)                                         | false    |              |             |
| `» model_config`       | array                                                | false    |              |             |
| `» owner_id`           | string(uuid)                                         | false    |              |             |
| `» status`             | [codersdk.ChatStatus](schemas.md#codersdkchatstatus) | false    |              |             |
| `» title`              | string                                               | false    |              |             |
| `» updated_at`         | string(date-time)                                    | false    |              |             |
| `» workspace_agent_id` | string(uuid)                                         | false    |              |             |
| `» workspace_id`       | string(uuid)                                         | false    |              |             |

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
      "role": "string",
      "thinking": "string",
      "tool_call_id": "string",
      "tool_calls": [
        0
      ]
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

## Get git changes for a chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/git-changes \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/git-changes`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
[
  {
    "change_type": "string",
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "detected_at": "2019-08-24T14:15:22Z",
    "diff_summary": "string",
    "file_path": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "old_path": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                              |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatGitChange](schemas.md#codersdkchatgitchange) |

<h3 id="get-git-changes-for-a-chat-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description                       |
|------------------|-------------------|----------|--------------|-----------------------------------|
| `[array item]`   | array             | false    |              |                                   |
| `» change_type`  | string            | false    |              | added, modified, deleted, renamed |
| `» chat_id`      | string(uuid)      | false    |              |                                   |
| `» detected_at`  | string(date-time) | false    |              |                                   |
| `» diff_summary` | string            | false    |              |                                   |
| `» file_path`    | string            | false    |              |                                   |
| `» id`           | string(uuid)      | false    |              |                                   |
| `» old_path`     | string            | false    |              |                                   |

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
  "tool_call_id": "string",
  "tool_calls": [
    0
  ]
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
    "role": "string",
    "thinking": "string",
    "tool_call_id": "string",
    "tool_calls": [
      0
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                          |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatMessage](schemas.md#codersdkchatmessage) |

<h3 id="create-a-chat-message-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description |
|------------------|-------------------|----------|--------------|-------------|
| `[array item]`   | array             | false    |              |             |
| `» chat_id`      | string(uuid)      | false    |              |             |
| `» content`      | array             | false    |              |             |
| `» created_at`   | string(date-time) | false    |              |             |
| `» hidden`       | boolean           | false    |              |             |
| `» id`           | integer           | false    |              |             |
| `» role`         | string            | false    |              |             |
| `» thinking`     | string            | false    |              |             |
| `» tool_call_id` | string            | false    |              |             |
| `» tool_calls`   | array             | false    |              |             |

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
