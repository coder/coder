# Applications

## Redirect to URI with encrypted API key

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/applications/auth-redirect \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /applications/auth-redirect`

### Parameters

| Name           | In    | Type   | Required | Description          |
|----------------|-------|--------|----------|----------------------|
| `redirect_uri` | query | string | false    | Redirect destination |

### Responses

| Status | Meaning                                                                 | Description        | Schema |
|--------|-------------------------------------------------------------------------|--------------------|--------|
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get applications host

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/applications/host \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /applications/host`

### Example responses

> 200 Response

```json
{
  "host": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AppHostResponse](schemas.md#codersdkapphostresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
