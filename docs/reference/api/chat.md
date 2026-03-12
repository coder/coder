# Chat

## Get per-user chat cost rollup

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/cost/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/cost/users`

### Parameters

| Name         | In    | Type    | Required | Description          |
|--------------|-------|---------|----------|----------------------|
| `start_date` | query | string  | false    | Start date (RFC3339) |
| `end_date`   | query | string  | false    | End date (RFC3339)   |
| `username`   | query | string  | false    | Filter by username   |
| `limit`      | query | integer | false    | Page size            |
| `offset`     | query | integer | false    | Page offset          |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "end_date": "2019-08-24T14:15:22Z",
  "start_date": "2019-08-24T14:15:22Z",
  "users": [
    {
      "avatar_url": "string",
      "chat_count": 0,
      "message_count": 0,
      "name": "string",
      "total_cost_micros": 0,
      "total_input_tokens": 0,
      "total_output_tokens": 0,
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
      "username": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatCostUsersResponse](schemas.md#codersdkchatcostusersresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat cost summary for a user

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/cost/{user}/summary \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/cost/{user}/summary`

### Parameters

| Name         | In    | Type   | Required | Description          |
|--------------|-------|--------|----------|----------------------|
| `user`       | path  | string | true     | User ID, name, or me |
| `start_date` | query | string | false    | Start date (RFC3339) |
| `end_date`   | query | string | false    | End date (RFC3339)   |

### Example responses

> 200 Response

```json
{
  "by_chat": [
    {
      "chat_title": "string",
      "message_count": 0,
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "total_cost_micros": 0,
      "total_input_tokens": 0,
      "total_output_tokens": 0
    }
  ],
  "by_model": [
    {
      "display_name": "string",
      "message_count": 0,
      "model": "string",
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
      "provider": "string",
      "total_cost_micros": 0,
      "total_input_tokens": 0,
      "total_output_tokens": 0
    }
  ],
  "end_date": "2019-08-24T14:15:22Z",
  "priced_message_count": 0,
  "start_date": "2019-08-24T14:15:22Z",
  "total_cost_micros": 0,
  "total_input_tokens": 0,
  "total_output_tokens": 0,
  "unpriced_message_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatCostSummary](schemas.md#codersdkchatcostsummary) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
