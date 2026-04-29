# Boundary

## Get boundary session logs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/boundary/sessions/{id}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /boundary/sessions/{id}/logs`

### Parameters

| Name         | In    | Type         | Required | Description                                    |
|--------------|-------|--------------|----------|------------------------------------------------|
| `id`         | path  | string(uuid) | true     | Boundary session ID                            |
| `seq_after`  | query | integer      | false    | Exclusive lower bound on sequence number       |
| `seq_before` | query | integer      | false    | Exclusive upper bound on sequence number       |
| `limit`      | query | integer      | false    | Maximum number of logs to return (default 100) |

### Example responses

> 200 Response

```json
{
  "results": [
    {
      "allowed": true,
      "captured_at": "2019-08-24T14:15:22Z",
      "detail": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "matched_rule": "string",
      "method": "string",
      "proto": "string",
      "sequence_number": 0,
      "session_id": "1ffd059c-17ea-40a8-8aef-70fd0307db82",
      "time": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                 |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.BoundarySessionLogsResponse](schemas.md#codersdkboundarysessionlogsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
