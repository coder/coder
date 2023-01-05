# Authorization

> This page is incomplete, stay tuned.

## Check authorization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/authcheck \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /authcheck`

> Body parameter

```json
{
  "checks": {
    "property1": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    }
  }
}
```

### Parameters

| Name   | In   | Type                                                                     | Required | Description           |
| ------ | ---- | ------------------------------------------------------------------------ | -------- | --------------------- |
| `body` | body | [codersdk.AuthorizationRequest](schemas.md#codersdkauthorizationrequest) | true     | Authorization request |

### Example responses

> 200 Response

```json
{
  "property1": true,
  "property2": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AuthorizationResponse](schemas.md#codersdkauthorizationresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
