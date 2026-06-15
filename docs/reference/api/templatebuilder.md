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

## List template builder modules

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templatebuilder/modules \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/templatebuilder/modules`

### Parameters

| Name   | In    | Type   | Required | Description                                             |
|--------|-------|--------|----------|---------------------------------------------------------|
| `base` | query | string | false    | Base template example ID for OS-compatibility filtering |

### Example responses

> 200 Response

```json
{
  "modules": [
    {
      "category": "string",
      "compatible_os": [
        "string"
      ],
      "conflicts_with": [
        "string"
      ],
      "description": "string",
      "display_name": "string",
      "icon": "string",
      "id": "string",
      "variables": [
        {
          "default": [
            0
          ],
          "description": "string",
          "name": "string",
          "required": true,
          "sensitive": true,
          "type": "string"
        }
      ],
      "version": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateBuilderModulesResponse](schemas.md#codersdktemplatebuildermodulesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
