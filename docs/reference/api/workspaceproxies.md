# WorkspaceProxies

## Get site-wide regions for workspace connections

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/regions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /regions`

### Example responses

> 200 Response

```json
{
  "regions": [
    {
      "display_name": "string",
      "healthy": true,
      "icon_url": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "name": "string",
      "path_app_url": "string",
      "wildcard_hostname": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                         |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.RegionsResponse-codersdk_Region](schemas.md#codersdkregionsresponse-codersdk_region) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
