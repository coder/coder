# Boundary

## Get boundary session by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/boundary/sessions/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/boundary/sessions/{id}`

### Parameters

| Name | In   | Type         | Required | Description         |
|------|------|--------------|----------|---------------------|
| `id` | path | string(uuid) | true     | Boundary session ID |

### Example responses

> 200 Response

```json
{
  "confined_process": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "started_at": "2019-08-24T14:15:22Z",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.BoundarySession](schemas.md#codersdkboundarysession) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
