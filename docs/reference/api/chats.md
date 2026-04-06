# Chats

## Get chat runtime summary

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/experimental/chats/runtime/summary \
  -H 'Accept: application/json'
```

`GET /experimental/chats/runtime/summary`

### Parameters

| Name         | In    | Type   | Required | Description          |
|--------------|-------|--------|----------|----------------------|
| `start_date` | query | string | false    | Start date (RFC3339) |
| `end_date`   | query | string | false    | End date (RFC3339)   |

### Example responses

> 200 Response

```json
{
  "daily": [
    {
      "date": "2019-08-24T14:15:22Z",
      "message_count": 0,
      "total_runtime_ms": 0
    }
  ],
  "end_date": "2019-08-24T14:15:22Z",
  "projected_yearly_runtime_ms": 0,
  "start_date": "2019-08-24T14:15:22Z",
  "total_runtime_ms": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatRuntimeSummary](schemas.md#codersdkchatruntimesummary) |
