# TemplateBuilder

## List template builder base templates

### Code samples

```sh
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
      "os": "string",
      "prerequisites": "string",
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
      ]
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateBuilderBasesResponse](schemas.md#codersdktemplatebuilderbasesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Compose template from base and modules

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templatebuilder/compose \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/templatebuilder/compose`

> Body parameter

```json
{
  "base_template_id": "string",
  "base_variable_values": {
    "property1": "string",
    "property2": "string"
  },
  "modules": [
    {
      "id": "string",
      "variables": {
        "property1": "string",
        "property2": "string"
      }
    }
  ]
}
```

### Parameters

| Name   | In   | Type                                                                                       | Required | Description     |
|--------|------|--------------------------------------------------------------------------------------------|----------|-----------------|
| `body` | body | [codersdk.TemplateBuilderComposeRequest](schemas.md#codersdktemplatebuildercomposerequest) | true     | Compose request |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Compose and create a template

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templatebuilder/compose/template \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/templatebuilder/compose/template`

> Body parameter

```json
{
  "base_template_id": "string",
  "base_variable_values": {
    "property1": "string",
    "property2": "string"
  },
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "modules": [
    {
      "id": "string",
      "variables": {
        "property1": "string",
        "property2": "string"
      }
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner_tags": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Parameters

| Name   | In   | Type                                                                                                     | Required | Description             |
|--------|------|----------------------------------------------------------------------------------------------------------|----------|-------------------------|
| `body` | body | [codersdk.TemplateBuilderCreateTemplateRequest](schemas.md#codersdktemplatebuildercreatetemplaterequest) | true     | Create template request |

### Example responses

> 201 Response

```json
{
  "template": {
    "active_user_count": 0,
    "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
    "activity_bump_ms": 0,
    "allow_user_autostart": true,
    "allow_user_autostop": true,
    "allow_user_cancel_workspace_jobs": true,
    "autostart_requirement": {
      "days_of_week": [
        "monday"
      ]
    },
    "autostop_requirement": {
      "days_of_week": [
        "monday"
      ],
      "weeks": 0
    },
    "build_time_stats": {
      "property1": {
        "p50": 123,
        "p95": 146
      },
      "property2": {
        "p50": 123,
        "p95": 146
      }
    },
    "cors_behavior": "simple",
    "created_at": "2019-08-24T14:15:22Z",
    "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
    "created_by_name": "string",
    "default_ttl_ms": 0,
    "deleted": true,
    "deprecated": true,
    "deprecation_message": "string",
    "description": "string",
    "disable_module_cache": true,
    "display_name": "string",
    "failure_ttl_ms": 0,
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "max_port_share_level": "owner",
    "name": "string",
    "organization_display_name": "string",
    "organization_icon": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_name": "string",
    "provisioner": "terraform",
    "require_active_version": true,
    "time_til_autostop_notify_ms": 0,
    "time_til_dormant_autodelete_ms": 0,
    "time_til_dormant_ms": 0,
    "updated_at": "2019-08-24T14:15:22Z",
    "use_classic_parameter_flow": true
  }
}
```

### Responses

| Status | Meaning                                                               | Description     | Schema                                                                                                     |
|--------|-----------------------------------------------------------------------|-----------------|------------------------------------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)          | Created         | [codersdk.TemplateBuilderCreateTemplateResponse](schemas.md#codersdktemplatebuildercreatetemplateresponse) |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)      | Bad Request     | [codersdk.Response](schemas.md#codersdkresponse)                                                           |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)        | Not Found       | [codersdk.Response](schemas.md#codersdkresponse)                                                           |
| 409    | [Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)         | Conflict        | [codersdk.Response](schemas.md#codersdkresponse)                                                           |
| 504    | [Gateway Time-out](https://tools.ietf.org/html/rfc7231#section-6.6.5) | Gateway Timeout | [codersdk.Response](schemas.md#codersdkresponse)                                                           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List template builder modules

### Code samples

```sh
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
