# AIBridge

## List AIBridge interceptions

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/api/experimental/aibridge/interceptions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/aibridge/interceptions`

### Parameters

| Name       | In    | Type    | Required | Description                                                                                                            |
|------------|-------|---------|----------|------------------------------------------------------------------------------------------------------------------------|
| `q`        | query | string  | false    | Search query in the format `key:value`. Available keys are: initiator, provider, model, started_after, started_before. |
| `limit`    | query | integer | false    | Page limit                                                                                                             |
| `after_id` | query | string  | false    | Cursor pagination after ID                                                                                             |

### Example responses

> 200 Response

```json
{
  "results": [
    {
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
      "metadata": {
        "property1": null,
        "property2": null
      },
      "model": "string",
      "provider": "string",
      "started_at": "2019-08-24T14:15:22Z",
      "token_usages": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "input_tokens": 0,
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "output_tokens": 0,
          "provider_response_id": "string"
        }
      ],
      "tool_usages": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "injected": true,
          "input": "string",
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "invocation_error": "string",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "provider_response_id": "string",
          "server_url": "string",
          "tool": "string"
        }
      ],
      "user_prompts": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "interception_id": "34d9b688-63ad-46f4-88b5-665c1e7f7824",
          "metadata": {
            "property1": null,
            "property2": null
          },
          "prompt": "string",
          "provider_response_id": "string"
        }
      ]
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIBridgeListInterceptionsResponse](schemas.md#codersdkaibridgelistinterceptionsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
