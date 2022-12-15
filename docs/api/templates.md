# Templates

## Create template by organization

### Code samples

```shell
# You can also use wget
curl -X POST http://coder-server:8080/api/v2/organizations/{organization-id}/templates/ \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`POST /organizations/{organization-id}/templates/`

> Body parameter

```json
{
  "allow_user_cancel_workspace_jobs": true,
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string",
  "parameter_values": [
    {
      "copy_from_parameter": "string",
      "destination_scheme": "environment_variable",
      "name": "string",
      "source_scheme": "data",
      "source_value": "string"
    }
  ],
  "template_version_id": "string"
}
```

### Parameters

| Name            | In   | Type                                                                       | Required | Description     |
| --------------- | ---- | -------------------------------------------------------------------------- | -------- | --------------- |
| organization-id | path | string                                                                     | true     | Organization ID |
| body            | body | [codersdk.CreateTemplateRequest](schemas.md#codersdkcreatetemplaterequest) | true     | Request body    |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "string",
  "allow_user_cancel_workspace_jobs": true,
  "build_time_stats": {
    "property1": {
      "p50": 0,
      "p95": 0
    },
    "property2": {
      "p50": 0,
      "p95": 0
    }
  },
  "created_at": "string",
  "created_by_id": "string",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "string",
  "name": "string",
  "organization_id": "string",
  "provisioner": "string",
  "updated_at": "string",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                                    | Description           | Schema                                           |
| ------ | -------------------------------------------------------------------------- | --------------------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | [codersdk.Template](schemas.md#codersdktemplate) |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | [codersdk.Response](schemas.md#codersdkresponse) |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template metadata by ID

### Code samples

```shell
# You can also use wget
curl -X GET http://coder-server:8080/api/v2/templates/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /templates/{id}`

### Parameters

| Name | In   | Type   | Required | Description |
| ---- | ---- | ------ | -------- | ----------- |
| id   | path | string | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "string",
  "allow_user_cancel_workspace_jobs": true,
  "build_time_stats": {
    "property1": {
      "p50": 0,
      "p95": 0
    },
    "property2": {
      "p50": 0,
      "p95": 0
    }
  },
  "created_at": "string",
  "created_by_id": "string",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "string",
  "name": "string",
  "organization_id": "string",
  "provisioner": "string",
  "updated_at": "string",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                                    | Description           | Schema                                           |
| ------ | -------------------------------------------------------------------------- | --------------------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | [codersdk.Template](schemas.md#codersdktemplate) |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | [codersdk.Response](schemas.md#codersdkresponse) |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
