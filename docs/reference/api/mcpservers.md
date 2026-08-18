# MCP Servers

## List MCP server configs

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/mcp-servers \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/mcp-servers`

### Example responses

> 200 Response

```json
[
  {
    "allow_in_plan_mode": true,
    "api_key_header": "string",
    "auth_connected": true,
    "auth_type": "string",
    "availability": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "description": "string",
    "display_name": "string",
    "enabled": true,
    "external_auth_provider_id": "string",
    "forward_coder_headers": true,
    "has_api_key": true,
    "has_custom_headers": true,
    "has_oauth2_secret": true,
    "icon_url": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "model_intent": true,
    "oauth2_auth_url": "string",
    "oauth2_client_id": "string",
    "oauth2_revocation_url": "string",
    "oauth2_scopes": "string",
    "oauth2_token_url": "string",
    "slug": "string",
    "tool_allow_list": [
      "string"
    ],
    "tool_default": "string",
    "tool_deny_list": [
      "string"
    ],
    "tool_rules": [
      {
        "enabled": true,
        "tool": "string"
      }
    ],
    "transport": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "url": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.MCPServerConfig](schemas.md#codersdkmcpserverconfig) |

<h3 id="list-mcp-server-configs-responseschema">Response Schema</h3>

Status Code **200**

| Name                          | Type              | Required | Restrictions | Description                                                                                                                                                                                                                                                                                          |
|-------------------------------|-------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                | array             | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» allow_in_plan_mode`        | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» api_key_header`            | string            | false    |              | Api key header key fields (only populated for admins).                                                                                                                                                                                                                                               |
| `» auth_connected`            | boolean           | false    |              | Per-user state (populated for non-admin requests).                                                                                                                                                                                                                                                   |
| `» auth_type`                 | string            | false    |              | "none", "oauth2", "api_key", "custom_headers", "user_oidc", "external_auth"                                                                                                                                                                                                                          |
| `» availability`              | string            | false    |              | Availability policy set by admin.                                                                                                                                                                                                                                                                    |
| `» created_at`                | string(date-time) | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» description`               | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» display_name`              | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» enabled`                   | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» external_auth_provider_id` | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» forward_coder_headers`     | boolean           | false    |              | Forward coder headers forwards the same Coder identity headers we send to LLM providers (X-Coder-Owner-Id, X-Coder-Chat-Id, and the optional X-Coder-Subchat-Id and X-Coder-Workspace-Id) to this MCP server on every request. Off by default to avoid leaking chat identity to third-party servers. |
| `» has_api_key`               | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» has_custom_headers`        | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» has_oauth2_secret`         | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» icon_url`                  | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» id`                        | string(uuid)      | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» model_intent`              | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» oauth2_auth_url`           | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» oauth2_client_id`          | string            | false    |              | Oauth2 client ID fields (only populated for admins).                                                                                                                                                                                                                                                 |
| `» oauth2_revocation_url`     | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» oauth2_scopes`             | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» oauth2_token_url`          | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» slug`                      | string            | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» tool_allow_list`           | array             | false    |              | Tool governance.                                                                                                                                                                                                                                                                                     |
| `» tool_default`              | string            | false    |              | "enabled" or "disabled"                                                                                                                                                                                                                                                                              |
| `» tool_deny_list`            | array             | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» tool_rules`                | array             | false    |              |                                                                                                                                                                                                                                                                                                      |
| `»» enabled`                  | boolean           | false    |              |                                                                                                                                                                                                                                                                                                      |
| `»» tool`                     | string            | true     |              |                                                                                                                                                                                                                                                                                                      |
| `» transport`                 | string            | false    |              | "streamable_http" or "sse"                                                                                                                                                                                                                                                                           |
| `» updated_at`                | string(date-time) | false    |              |                                                                                                                                                                                                                                                                                                      |
| `» url`                       | string            | false    |              |                                                                                                                                                                                                                                                                                                      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an MCP server config

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/ai-gateway/mcp-servers \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/ai-gateway/mcp-servers`

> Body parameter

```json
{
  "allow_in_plan_mode": true,
  "api_key_header": "string",
  "api_key_value": "string",
  "auth_type": "none",
  "availability": "force_on",
  "custom_headers": {
    "property1": "string",
    "property2": "string"
  },
  "description": "string",
  "display_name": "string",
  "enabled": true,
  "external_auth_provider_id": "string",
  "forward_coder_headers": true,
  "icon_url": "string",
  "model_intent": true,
  "oauth2_auth_url": "string",
  "oauth2_client_id": "string",
  "oauth2_client_secret": "string",
  "oauth2_revocation_url": "string",
  "oauth2_scopes": "string",
  "oauth2_token_url": "string",
  "slug": "string",
  "tool_allow_list": [
    "string"
  ],
  "tool_default": "enabled",
  "tool_deny_list": [
    "string"
  ],
  "tool_rules": [
    {
      "enabled": true,
      "tool": "string"
    }
  ],
  "transport": "streamable_http",
  "url": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                     | Required | Description                      |
|--------|------|------------------------------------------------------------------------------------------|----------|----------------------------------|
| `body` | body | [codersdk.CreateMCPServerConfigRequest](schemas.md#codersdkcreatemcpserverconfigrequest) | true     | Create MCP server config request |

### Example responses

> 201 Response

```json
{
  "allow_in_plan_mode": true,
  "api_key_header": "string",
  "auth_connected": true,
  "auth_type": "string",
  "availability": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "enabled": true,
  "external_auth_provider_id": "string",
  "forward_coder_headers": true,
  "has_api_key": true,
  "has_custom_headers": true,
  "has_oauth2_secret": true,
  "icon_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "model_intent": true,
  "oauth2_auth_url": "string",
  "oauth2_client_id": "string",
  "oauth2_revocation_url": "string",
  "oauth2_scopes": "string",
  "oauth2_token_url": "string",
  "slug": "string",
  "tool_allow_list": [
    "string"
  ],
  "tool_default": "string",
  "tool_deny_list": [
    "string"
  ],
  "tool_rules": [
    {
      "enabled": true,
      "tool": "string"
    }
  ],
  "transport": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                         |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.MCPServerConfig](schemas.md#codersdkmcpserverconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get an MCP server config

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/mcp-servers/{mcpServer}`

### Parameters

| Name        | In   | Type         | Required | Description          |
|-------------|------|--------------|----------|----------------------|
| `mcpServer` | path | string(uuid) | true     | MCP server config ID |

### Example responses

> 200 Response

```json
{
  "allow_in_plan_mode": true,
  "api_key_header": "string",
  "auth_connected": true,
  "auth_type": "string",
  "availability": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "enabled": true,
  "external_auth_provider_id": "string",
  "forward_coder_headers": true,
  "has_api_key": true,
  "has_custom_headers": true,
  "has_oauth2_secret": true,
  "icon_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "model_intent": true,
  "oauth2_auth_url": "string",
  "oauth2_client_id": "string",
  "oauth2_revocation_url": "string",
  "oauth2_scopes": "string",
  "oauth2_token_url": "string",
  "slug": "string",
  "tool_allow_list": [
    "string"
  ],
  "tool_default": "string",
  "tool_deny_list": [
    "string"
  ],
  "tool_rules": [
    {
      "enabled": true,
      "tool": "string"
    }
  ],
  "transport": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.MCPServerConfig](schemas.md#codersdkmcpserverconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an MCP server config

### Code samples

```sh
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/ai-gateway/mcp-servers/{mcpServer}`

### Parameters

| Name        | In   | Type         | Required | Description          |
|-------------|------|--------------|----------|----------------------|
| `mcpServer` | path | string(uuid) | true     | MCP server config ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update an MCP server config

### Code samples

```sh
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/ai-gateway/mcp-servers/{mcpServer}`

> Body parameter

```json
{
  "allow_in_plan_mode": true,
  "api_key_header": "string",
  "api_key_value": "string",
  "auth_type": "none",
  "availability": "force_on",
  "custom_headers": {
    "property1": "string",
    "property2": "string"
  },
  "description": "string",
  "display_name": "string",
  "enabled": true,
  "external_auth_provider_id": "string",
  "forward_coder_headers": true,
  "icon_url": "string",
  "model_intent": true,
  "oauth2_auth_url": "string",
  "oauth2_client_id": "string",
  "oauth2_client_secret": "string",
  "oauth2_revocation_url": "string",
  "oauth2_scopes": "string",
  "oauth2_token_url": "string",
  "slug": "string",
  "tool_allow_list": [
    "string"
  ],
  "tool_default": "enabled",
  "tool_deny_list": [
    "string"
  ],
  "tool_rules": [
    {
      "enabled": true,
      "tool": "string"
    }
  ],
  "transport": "streamable_http",
  "url": "string"
}
```

### Parameters

| Name        | In   | Type                                                                                     | Required | Description                      |
|-------------|------|------------------------------------------------------------------------------------------|----------|----------------------------------|
| `mcpServer` | path | string(uuid)                                                                             | true     | MCP server config ID             |
| `body`      | body | [codersdk.UpdateMCPServerConfigRequest](schemas.md#codersdkupdatemcpserverconfigrequest) | true     | Update MCP server config request |

### Example responses

> 200 Response

```json
{
  "allow_in_plan_mode": true,
  "api_key_header": "string",
  "auth_connected": true,
  "auth_type": "string",
  "availability": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "enabled": true,
  "external_auth_provider_id": "string",
  "forward_coder_headers": true,
  "has_api_key": true,
  "has_custom_headers": true,
  "has_oauth2_secret": true,
  "icon_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "model_intent": true,
  "oauth2_auth_url": "string",
  "oauth2_client_id": "string",
  "oauth2_revocation_url": "string",
  "oauth2_scopes": "string",
  "oauth2_token_url": "string",
  "slug": "string",
  "tool_allow_list": [
    "string"
  ],
  "tool_default": "string",
  "tool_deny_list": [
    "string"
  ],
  "tool_rules": [
    {
      "enabled": true,
      "tool": "string"
    }
  ],
  "transport": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.MCPServerConfig](schemas.md#codersdkmcpserverconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Handle MCP server OAuth2 callback

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/callback \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/callback`

### Parameters

| Name        | In   | Type         | Required | Description          |
|-------------|------|--------------|----------|----------------------|
| `mcpServer` | path | string(uuid) | true     | MCP server config ID |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Initiate MCP server OAuth2 connect

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/connect \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/connect`

### Parameters

| Name        | In   | Type         | Required | Description          |
|-------------|------|--------------|----------|----------------------|
| `mcpServer` | path | string(uuid) | true     | MCP server config ID |

### Responses

| Status | Meaning                                                                 | Description        | Schema |
|--------|-------------------------------------------------------------------------|--------------------|--------|
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Disconnect MCP server OAuth2 token

### Code samples

```sh
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/disconnect \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/ai-gateway/mcp-servers/{mcpServer}/oauth2/disconnect`

### Parameters

| Name        | In   | Type         | Required | Description          |
|-------------|------|--------------|----------|----------------------|
| `mcpServer` | path | string(uuid) | true     | MCP server config ID |

### Example responses

> 200 Response

```json
{
  "token_revocation_error": "string",
  "token_revoked": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.MCPServerOAuth2DisconnectResponse](schemas.md#codersdkmcpserveroauth2disconnectresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
