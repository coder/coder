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
| `after_id` | query | string  | false    | Cursor pagination after ID (cannot be used with offset)                                                                |
| `offset`   | query | integer | false    | Offset pagination (cannot be used with after_id)                                                                       |

### Example responses

> 200 Response

```json
{
  "results": [
    {
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "initiator": {
        "avatar_url": "http://example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "name": "string",
        "username": "string"
      },
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
  ],
  "total": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIBridgeListInterceptionsResponse](schemas.md#codersdkaibridgelistinterceptionsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Set AI Bridge request logging

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/api/experimental/aibridge/log-requests \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/aibridge/log-requests`

> Body parameter

```json
{
  "enabled": true
}
```

### Parameters

| Name   | In   | Type                                                                                             | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.AIBridgeSetRequestLoggingRequest](schemas.md#codersdkaibridgesetrequestloggingrequest) | true     | Request body |

### Example responses

> 200 Response

```json
{
  "detail": "string",
  "message": "string",
  "validations": [
    {
      "detail": "string",
      "field": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
