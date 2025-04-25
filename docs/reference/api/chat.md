# Chat

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
    "title": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                            |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Chat](schemas.md#codersdkchat) |

<h3 id="list-chats-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type              | Required | Restrictions | Description |
|----------------|-------------------|----------|--------------|-------------|
| `[array item]` | array             | false    |              |             |
| `» created_at` | string(date-time) | false    |              |             |
| `» id`         | string(uuid)      | false    |              |             |
| `» title`      | string            | false    |              |             |
| `» updated_at` | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats`

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Chat](schemas.md#codersdkchat) |

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

| Name   | In   | Type   | Required | Description |
|--------|------|--------|----------|-------------|
| `chat` | path | string | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat messages

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
    "annotations": [
      null
    ],
    "content": "string",
    "createdAt": [
      0
    ],
    "experimental_attachments": [
      {
        "contentType": "string",
        "name": "string",
        "url": "string"
      }
    ],
    "id": "string",
    "parts": [
      {
        "data": "string",
        "details": [
          {
            "data": "string",
            "signature": "string",
            "text": "string",
            "type": "string"
          }
        ],
        "mimeType": "string",
        "reasoning": "string",
        "source": {
          "contentType": "string",
          "data": "string",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "uri": "string"
        },
        "text": "string",
        "toolInvocation": {
          "args": null,
          "result": null,
          "state": "call",
          "step": 0,
          "toolCallId": "string",
          "toolName": "string"
        },
        "type": "text"
      }
    ],
    "role": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                            |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [aisdk.Message](schemas.md#aisdkmessage) |

<h3 id="get-chat-messages-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                             | Required | Restrictions | Description             |
|------------------------------|------------------------------------------------------------------|----------|--------------|-------------------------|
| `[array item]`               | array                                                            | false    |              |                         |
| `» annotations`              | array                                                            | false    |              |                         |
| `» content`                  | string                                                           | false    |              |                         |
| `» createdAt`                | array                                                            | false    |              |                         |
| `» experimental_attachments` | array                                                            | false    |              |                         |
| `»» contentType`             | string                                                           | false    |              |                         |
| `»» name`                    | string                                                           | false    |              |                         |
| `»» url`                     | string                                                           | false    |              |                         |
| `» id`                       | string                                                           | false    |              |                         |
| `» parts`                    | array                                                            | false    |              |                         |
| `»» data`                    | string                                                           | false    |              |                         |
| `»» details`                 | array                                                            | false    |              |                         |
| `»»» data`                   | string                                                           | false    |              |                         |
| `»»» signature`              | string                                                           | false    |              |                         |
| `»»» text`                   | string                                                           | false    |              |                         |
| `»»» type`                   | string                                                           | false    |              |                         |
| `»» mimeType`                | string                                                           | false    |              | Type: "file"            |
| `»» reasoning`               | string                                                           | false    |              | Type: "reasoning"       |
| `»» source`                  | [aisdk.SourceInfo](schemas.md#aisdksourceinfo)                   | false    |              | Type: "source"          |
| `»»» contentType`            | string                                                           | false    |              |                         |
| `»»» data`                   | string                                                           | false    |              |                         |
| `»»» metadata`               | object                                                           | false    |              |                         |
| `»»»» [any property]`        | any                                                              | false    |              |                         |
| `»»» uri`                    | string                                                           | false    |              |                         |
| `»» text`                    | string                                                           | false    |              | Type: "text"            |
| `»» toolInvocation`          | [aisdk.ToolInvocation](schemas.md#aisdktoolinvocation)           | false    |              | Type: "tool-invocation" |
| `»»» args`                   | any                                                              | false    |              |                         |
| `»»» result`                 | any                                                              | false    |              |                         |
| `»»» state`                  | [aisdk.ToolInvocationState](schemas.md#aisdktoolinvocationstate) | false    |              |                         |
| `»»» step`                   | integer                                                          | false    |              |                         |
| `»»» toolCallId`             | string                                                           | false    |              |                         |
| `»»» toolName`               | string                                                           | false    |              |                         |
| `»» type`                    | [aisdk.PartType](schemas.md#aisdkparttype)                       | false    |              |                         |
| `» role`                     | string                                                           | false    |              |                         |

#### Enumerated Values

| Property | Value             |
|----------|-------------------|
| `state`  | `call`            |
| `state`  | `partial-call`    |
| `state`  | `result`          |
| `type`   | `text`            |
| `type`   | `reasoning`       |
| `type`   | `tool-invocation` |
| `type`   | `source`          |
| `type`   | `file`            |
| `type`   | `step-start`      |

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
  "message": {
    "annotations": [
      null
    ],
    "content": "string",
    "createdAt": [
      0
    ],
    "experimental_attachments": [
      {
        "contentType": "string",
        "name": "string",
        "url": "string"
      }
    ],
    "id": "string",
    "parts": [
      {
        "data": "string",
        "details": [
          {
            "data": "string",
            "signature": "string",
            "text": "string",
            "type": "string"
          }
        ],
        "mimeType": "string",
        "reasoning": "string",
        "source": {
          "contentType": "string",
          "data": "string",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "uri": "string"
        },
        "text": "string",
        "toolInvocation": {
          "args": null,
          "result": null,
          "state": "call",
          "step": 0,
          "toolCallId": "string",
          "toolName": "string"
        },
        "type": "text"
      }
    ],
    "role": "string"
  },
  "model": "string",
  "thinking": true
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description  |
|--------|------|----------------------------------------------------------------------------------|----------|--------------|
| `chat` | path | string                                                                           | true     | Chat ID      |
| `body` | body | [codersdk.CreateChatMessageRequest](schemas.md#codersdkcreatechatmessagerequest) | true     | Request body |

### Example responses

> 200 Response

```json
[
  null
]
```

### Responses

| Status | Meaning                                                 | Description | Schema             |
|--------|---------------------------------------------------------|-------------|--------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of undefined |

<h3 id="create-a-chat-message-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).
