# AITasks

## Get AI tasks prompts

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aitasks/prompts?build_ids=497f6eca-6276-4993-bfeb-53cbbbba6f08 \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /aitasks/prompts`

### Parameters

| Name        | In    | Type         | Required | Description                         |
|-------------|-------|--------------|----------|-------------------------------------|
| `build_ids` | query | string(uuid) | true     | Comma-separated workspace build IDs |

### Example responses

> 200 Response

```json
{
  "prompts": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AITasksPromptsResponse](schemas.md#codersdkaitaskspromptsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
