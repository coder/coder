# Chat

## List chat MCP server configs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/mcp-servers \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/mcp-servers`

### Example responses

> 200 Response

```json
[
  {
    "auth_type": "none",
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "enabled": true,
    "has_auth_headers": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "slug": "string",
    "tool_allow_regex": "string",
    "tool_deny_regex": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "url": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                          |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatMCPServerConfig](schemas.md#codersdkchatmcpserverconfig) |

<h3 id="list-chat-mcp-server-configs-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                                       | Required | Restrictions | Description |
|----------------------|----------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`       | array                                                                      | false    |              |             |
| `» auth_type`        | [codersdk.ChatMCPServerAuthType](schemas.md#codersdkchatmcpserverauthtype) | false    |              |             |
| `» created_at`       | string(date-time)                                                          | false    |              |             |
| `» display_name`     | string                                                                     | false    |              |             |
| `» enabled`          | boolean                                                                    | false    |              |             |
| `» has_auth_headers` | boolean                                                                    | false    |              |             |
| `» id`               | string(uuid)                                                               | false    |              |             |
| `» slug`             | string                                                                     | false    |              |             |
| `» tool_allow_regex` | string                                                                     | false    |              |             |
| `» tool_deny_regex`  | string                                                                     | false    |              |             |
| `» updated_at`       | string(date-time)                                                          | false    |              |             |
| `» url`              | string                                                                     | false    |              |             |

#### Enumerated Values

| Property    | Value(s)                  |
|-------------|---------------------------|
| `auth_type` | `header`, `none`, `oauth` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat MCP server config

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/mcp-servers \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats/mcp-servers`

> Body parameter

```json
{
  "auth_headers": {
    "property1": "string",
    "property2": "string"
  },
  "auth_type": "none",
  "display_name": "string",
  "enabled": true,
  "slug": "string",
  "tool_allow_regex": "string",
  "tool_deny_regex": "string",
  "url": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                 | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.CreateChatMCPServerRequest](schemas.md#codersdkcreatechatmcpserverrequest) | true     | Request body |

### Example responses

> 201 Response

```json
{
  "auth_type": "none",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "has_auth_headers": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "slug": "string",
  "tool_allow_regex": "string",
  "tool_deny_regex": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                 |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.ChatMCPServerConfig](schemas.md#codersdkchatmcpserverconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete chat MCP server config

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/chats/mcp-servers/{mcpServer} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /chats/mcp-servers/{mcpServer}`

### Parameters

| Name        | In   | Type         | Required | Description   |
|-------------|------|--------------|----------|---------------|
| `mcpServer` | path | string(uuid) | true     | MCP Server ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat MCP server config

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/chats/mcp-servers/{mcpServer} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /chats/mcp-servers/{mcpServer}`

> Body parameter

```json
{
  "auth_headers": {
    "property1": "string",
    "property2": "string"
  },
  "auth_type": "none",
  "display_name": "string",
  "enabled": true,
  "slug": "string",
  "tool_allow_regex": "string",
  "tool_deny_regex": "string",
  "url": "string"
}
```

### Parameters

| Name        | In   | Type                                                                                 | Required | Description   |
|-------------|------|--------------------------------------------------------------------------------------|----------|---------------|
| `mcpServer` | path | string(uuid)                                                                         | true     | MCP Server ID |
| `body`      | body | [codersdk.UpdateChatMCPServerRequest](schemas.md#codersdkupdatechatmcpserverrequest) | true     | Request body  |

### Example responses

> 200 Response

```json
{
  "auth_type": "none",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "has_auth_headers": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "slug": "string",
  "tool_allow_regex": "string",
  "tool_deny_regex": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                 |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatMCPServerConfig](schemas.md#codersdkchatmcpserverconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
