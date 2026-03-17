# Chats

## Archive a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/archive

```

`POST /chats/{chat}/archive`

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

## Watch chat desktop

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/desktop \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/desktop`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
|--------|--------------------------------------------------------------------------|---------------------|--------|
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
