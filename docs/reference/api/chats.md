# Chats

## Create chat

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
  "metadata": [
    0
  ],
  "model": "string",
  "provider": "string",
  "title": "string",
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
  "created_at": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "metadata": [
    0
  ],
  "model": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "provider": "string",
  "title": "string",
  "updated_at": "string",
  "workspace_id": {
    "uuid": "string",
    "valid": true
  }
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}`

### Parameters

| Name   | In   | Type   | Required | Description |
|--------|------|--------|----------|-------------|
| `chat` | path | string | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "created_at": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "metadata": [
    0
  ],
  "model": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "provider": "string",
  "title": "string",
  "updated_at": "string",
  "workspace_id": {
    "uuid": "string",
    "valid": true
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat messages

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/messages \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/messages`

### Parameters

| Name   | In   | Type   | Required | Description |
|--------|------|--------|----------|-------------|
| `chat` | path | string | true     | Chat ID     |

### Example responses

> 200 Response

```json
[
  {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      0
    ],
    "created_at": "string",
    "id": 0,
    "role": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                          |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatMessage](schemas.md#codersdkchatmessage) |

<h3 id="list-chat-messages-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type         | Required | Restrictions | Description |
|----------------|--------------|----------|--------------|-------------|
| `[array item]` | array        | false    |              |             |
| `» chat_id`    | string(uuid) | false    |              |             |
| `» content`    | array        | false    |              |             |
| `» created_at` | string       | false    |              |             |
| `» id`         | integer      | false    |              |             |
| `» role`       | string       | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat message and run agent loop

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
  "content": "string"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description            |
|--------|------|----------------------------------------------------------------------------------|----------|------------------------|
| `chat` | path | string                                                                           | true     | Chat ID                |
| `body` | body | [codersdk.CreateChatMessageRequest](schemas.md#codersdkcreatechatmessagerequest) | true     | Create message request |

### Example responses

> 201 Response

```json
{
  "message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      0
    ],
    "created_at": "string",
    "id": 0,
    "role": "string"
  },
  "run_id": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                             |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.CreateChatMessageResponse](schemas.md#codersdkcreatechatmessageresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
