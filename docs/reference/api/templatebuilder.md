# TemplateBuilder

## List template builder base templates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templatebuilder/bases \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/templatebuilder/bases`

### Example responses

> 200 Response

```json
{
  "bases": [
    {
      "description": "string",
      "icon": "string",
      "id": "string",
      "name": "string",
      "os": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateBuilderBasesResponse](schemas.md#codersdktemplatebuilderbasesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
