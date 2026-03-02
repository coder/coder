# Chat

## Get chat config settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/config \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/config`

### Example responses

> 200 Response

```json
{
  "system_prompt": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatConfigSettings](schemas.md#codersdkchatconfigsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat config settings

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/chats/config \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /chats/config`

> Body parameter

```json
{
  "system_prompt": "string"
}
```

### Parameters

| Name   | In   | Type                                                                 | Required | Description                  |
|--------|------|----------------------------------------------------------------------|----------|------------------------------|
| `body` | body | [codersdk.ChatConfigSettings](schemas.md#codersdkchatconfigsettings) | true     | Chat config settings request |

### Example responses

> 200 Response

```json
{
  "system_prompt": "string"
}
```

### Responses

| Status | Meaning                                                         | Description  | Schema                                                               |
|--------|-----------------------------------------------------------------|--------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)         | OK           | [codersdk.ChatConfigSettings](schemas.md#codersdkchatconfigsettings) |
| 304    | [Not Modified](https://tools.ietf.org/html/rfc7232#section-4.1) | Not Modified |                                                                      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
