# Templates

> This page is incomplete, stay tuned.

## Create template by organization

### Code samples

```shell
# Example request using curl
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
      "p50": 123,
      "p95": 146
    },
    "property2": {
      "p50": 123,
      "p95": 146
    }
  },
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get templates by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates`

### Parameters

| Name         | In   | Type         | Required | Description     |
| ------------ | ---- | ------------ | -------- | --------------- |
| organization | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "active_user_count": 0,
    "active_version_id": "string",
    "allow_user_cancel_workspace_jobs": true,
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
    "created_at": "2019-08-24T14:15:22Z",
    "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
    "created_by_name": "string",
    "default_ttl_ms": 0,
    "description": "string",
    "display_name": "string",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "provisioner": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_owner_count": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                    |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Template](schemas.md#codersdktemplate) |

<h3 id="get-templates-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                               | Type                              | Required | Restrictions | Description                                |
| ---------------------------------- | --------------------------------- | -------- | ------------ | ------------------------------------------ |
| _anonymous_                        | array                             | false    |              |                                            |
| » active_user_count                | integer                           | false    |              | ActiveUserCount is set to -1 when loading. |
| » active_version_id                | string                            | false    |              |                                            |
| » allow_user_cancel_workspace_jobs | boolean                           | false    |              |                                            |
| » build_time_stats                 | `codersdk.TemplateBuildTimeStats` | false    |              |                                            |
| »» **additionalProperties**        | `codersdk.TransitionStats`        | false    |              |                                            |
| »»» p50                            | integer                           | false    |              |                                            |
| »»» p95                            | integer                           | false    |              |                                            |
| » created_at                       | string                            | false    |              |                                            |
| » created_by_id                    | string                            | false    |              |                                            |
| » created_by_name                  | string                            | false    |              |                                            |
| » default_ttl_ms                   | integer                           | false    |              |                                            |
| » description                      | string                            | false    |              |                                            |
| » display_name                     | string                            | false    |              |                                            |
| » icon                             | string                            | false    |              |                                            |
| » id                               | string                            | false    |              |                                            |
| » name                             | string                            | false    |              |                                            |
| » organization_id                  | string                            | false    |              |                                            |
| » provisioner                      | string                            | false    |              |                                            |
| » updated_at                       | string                            | false    |              |                                            |
| » workspace_owner_count            | integer                           | false    |              |                                            |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get templates by organization and template name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates/{template-name} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates/{template-name}`

### Parameters

| Name          | In   | Type         | Required | Description     |
| ------------- | ---- | ------------ | -------- | --------------- |
| organization  | path | string(uuid) | true     | Organization ID |
| template-name | path | string       | true     | Template name   |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "string",
  "allow_user_cancel_workspace_jobs": true,
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Update template metadata by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| id   | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "string",
  "allow_user_cancel_workspace_jobs": true,
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Delete template by ID

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/templates/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /templates/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| id   | path | string(uuid) | true     | Template ID |

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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
