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

## Unarchive a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/unarchive

```

`POST /chats/{chat}/unarchive`

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |
