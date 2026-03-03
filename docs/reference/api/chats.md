# Chats

## Get chat stats

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/stats?start_time=string&end_time=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/stats`

### Parameters

| Name         | In    | Type   | Required | Description                      |
|--------------|-------|--------|----------|----------------------------------|
| `start_time` | query | string | true     | Start time (RFC 3339, inclusive) |
| `end_time`   | query | string | true     | End time (RFC 3339, exclusive)   |

### Example responses

> 200 Response

```json
{
  "active_users": 0,
  "by_status": {
    "completed": 0,
    "error": 0,
    "paused": 0,
    "pending": 0,
    "running": 0,
    "waiting": 0
  },
  "end_time": "2019-08-24T14:15:22Z",
  "start_time": "2019-08-24T14:15:22Z",
  "total_assistant_messages": 0,
  "total_cache_creation_tokens": 0,
  "total_cache_read_tokens": 0,
  "total_chats": 0,
  "total_input_tokens": 0,
  "total_messages": 0,
  "total_output_tokens": 0,
  "total_reasoning_tokens": 0,
  "total_sub_chats": 0,
  "total_user_messages": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatStatsResponse](schemas.md#codersdkchatstatsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
