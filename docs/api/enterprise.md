# Enterprise

> This page is incomplete, stay tuned.

## Get entitlements

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/entitlements \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /entitlements`

### Example responses

> 200 Response

```json
{
  "errors": ["string"],
  "experimental": true,
  "features": {
    "property1": {
      "actual": 0,
      "enabled": true,
      "entitlement": "string",
      "limit": 0
    },
    "property2": {
      "actual": 0,
      "enabled": true,
      "entitlement": "string",
      "limit": 0
    }
  },
  "has_license": true,
  "trial": true,
  "warnings": ["string"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Entitlements](schemas.md#codersdkentitlements) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
