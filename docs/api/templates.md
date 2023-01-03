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
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
  "template_version_id": "string"
}
```

### Parameters

| Name              | In   | Type                                                                       | Required | Description     |
| ----------------- | ---- | -------------------------------------------------------------------------- | -------- | --------------- |
| `organization-id` | path | string                                                                     | true     | Organization ID |
| `body`            | body | [codersdk.CreateTemplateRequest](schemas.md#codersdkcreatetemplaterequest) | true     | Request body    |

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

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

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

| Name                                 | Type                              | Required | Restrictions | Description                                |
| ------------------------------------ | --------------------------------- | -------- | ------------ | ------------------------------------------ |
| `[array item]`                       | array                             | false    |              |                                            |
| `» active_user_count`                | integer                           | false    |              | ActiveUserCount is set to -1 when loading. |
| `» active_version_id`                | string                            | false    |              |                                            |
| `» allow_user_cancel_workspace_jobs` | boolean                           | false    |              |                                            |
| `» build_time_stats`                 | `codersdk.TemplateBuildTimeStats` | false    |              |                                            |
| `»» [any property]`                  | `codersdk.TransitionStats`        | false    |              |                                            |
| `»»» p50`                            | integer                           | false    |              |                                            |
| `»»» p95`                            | integer                           | false    |              |                                            |
| `» created_at`                       | string                            | false    |              |                                            |
| `» created_by_id`                    | string                            | false    |              |                                            |
| `» created_by_name`                  | string                            | false    |              |                                            |
| `» default_ttl_ms`                   | integer                           | false    |              |                                            |
| `» description`                      | string                            | false    |              |                                            |
| `» display_name`                     | string                            | false    |              |                                            |
| `» icon`                             | string                            | false    |              |                                            |
| `» id`                               | string                            | false    |              |                                            |
| `» name`                             | string                            | false    |              |                                            |
| `» organization_id`                  | string                            | false    |              |                                            |
| `» provisioner`                      | string                            | false    |              |                                            |
| `» updated_at`                       | string                            | false    |              |                                            |
| `» workspace_owner_count`            | integer                           | false    |              |                                            |

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

| Name            | In   | Type         | Required | Description     |
| --------------- | ---- | ------------ | -------- | --------------- |
| `organization`  | path | string(uuid) | true     | Organization ID |
| `template-name` | path | string       | true     | Template name   |

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

## Get template metadata by ID

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
| `id` | path | string(uuid) | true     | Template ID |

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
| `id` | path | string(uuid) | true     | Template ID |

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

## Update template metadata by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templates/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templates/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| `id` | path | string(uuid) | true     | Template ID |

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

## Get template DAUs by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{id}/daus \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{id}/daus`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| `id` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateDAUsResponse](schemas.md#codersdktemplatedausresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## List template versions by template ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{id}/versions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{id}/versions`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| `id` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job": {
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "readme": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

<h3 id="list-template-versions-by-template-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type                      | Required | Restrictions | Description |
| --------------------- | ------------------------- | -------- | ------------ | ----------- |
| `[array item]`        | array                     | false    |              |             |
| `» created_at`        | string                    | false    |              |             |
| `» created_by`        | `codersdk.User`           | false    |              |             |
| `»» avatar_url`       | string                    | false    |              |             |
| `»» created_at`       | string                    | true     |              |             |
| `»» email`            | string                    | true     |              |             |
| `»» id`               | string                    | true     |              |             |
| `»» last_seen_at`     | string                    | false    |              |             |
| `»» organization_ids` | array                     | false    |              |             |
| `»» roles`            | array                     | false    |              |             |
| `»»» display_name`    | string                    | false    |              |             |
| `»»» name`            | string                    | false    |              |             |
| `»» status`           | string                    | false    |              |             |
| `»» username`         | string                    | true     |              |             |
| `» id`                | string                    | false    |              |             |
| `» job`               | `codersdk.ProvisionerJob` | false    |              |             |
| `»» canceled_at`      | string                    | false    |              |             |
| `»» completed_at`     | string                    | false    |              |             |
| `»» created_at`       | string                    | false    |              |             |
| `»» error`            | string                    | false    |              |             |
| `»» file_id`          | string                    | false    |              |             |
| `»» id`               | string                    | false    |              |             |
| `»» started_at`       | string                    | false    |              |             |
| `»» status`           | string                    | false    |              |             |
| `»» tags`             | object                    | false    |              |             |
| `»»» [any property]`  | string                    | false    |              |             |
| `»» worker_id`        | string                    | false    |              |             |
| `» name`              | string                    | false    |              |             |
| `» organization_id`   | string                    | false    |              |             |
| `» readme`            | string                    | false    |              |             |
| `» template_id`       | string                    | false    |              |             |
| `» updated_at`        | string                    | false    |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `active`    |
| `status` | `suspended` |
| `status` | `pending`   |
| `status` | `running`   |
| `status` | `succeeded` |
| `status` | `canceling` |
| `status` | `canceled`  |
| `status` | `failed`    |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Update active template version by template ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templates/{id}/versions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templates/{id}/versions`

> Body parameter

```json
{
  "id": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description               |
| ------ | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------- |
| `id`   | path | string(uuid)                                                                           | true     | Template ID               |
| `body` | body | [codersdk.UpdateActiveTemplateVersion](schemas.md#codersdkupdateactivetemplateversion) | true     | Modified template version |

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

## Get template version by template ID and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{id}/versions/{name} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{id}/versions/{name}`

### Parameters

| Name   | In   | Type         | Required | Description   |
| ------ | ---- | ------------ | -------- | ------------- |
| `id`   | path | string(uuid) | true     | Template ID   |
| `name` | path | string       | true     | Template name |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job": {
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "readme": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

<h3 id="get-template-version-by-template-id-and-name-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type                      | Required | Restrictions | Description |
| --------------------- | ------------------------- | -------- | ------------ | ----------- |
| `[array item]`        | array                     | false    |              |             |
| `» created_at`        | string                    | false    |              |             |
| `» created_by`        | `codersdk.User`           | false    |              |             |
| `»» avatar_url`       | string                    | false    |              |             |
| `»» created_at`       | string                    | true     |              |             |
| `»» email`            | string                    | true     |              |             |
| `»» id`               | string                    | true     |              |             |
| `»» last_seen_at`     | string                    | false    |              |             |
| `»» organization_ids` | array                     | false    |              |             |
| `»» roles`            | array                     | false    |              |             |
| `»»» display_name`    | string                    | false    |              |             |
| `»»» name`            | string                    | false    |              |             |
| `»» status`           | string                    | false    |              |             |
| `»» username`         | string                    | true     |              |             |
| `» id`                | string                    | false    |              |             |
| `» job`               | `codersdk.ProvisionerJob` | false    |              |             |
| `»» canceled_at`      | string                    | false    |              |             |
| `»» completed_at`     | string                    | false    |              |             |
| `»» created_at`       | string                    | false    |              |             |
| `»» error`            | string                    | false    |              |             |
| `»» file_id`          | string                    | false    |              |             |
| `»» id`               | string                    | false    |              |             |
| `»» started_at`       | string                    | false    |              |             |
| `»» status`           | string                    | false    |              |             |
| `»» tags`             | object                    | false    |              |             |
| `»»» [any property]`  | string                    | false    |              |             |
| `»» worker_id`        | string                    | false    |              |             |
| `» name`              | string                    | false    |              |             |
| `» organization_id`   | string                    | false    |              |             |
| `» readme`            | string                    | false    |              |             |
| `» template_id`       | string                    | false    |              |             |
| `» updated_at`        | string                    | false    |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `active`    |
| `status` | `suspended` |
| `status` | `pending`   |
| `status` | `running`   |
| `status` | `succeeded` |
| `status` | `canceling` |
| `status` | `canceled`  |
| `status` | `failed`    |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template version by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{id}`

### Parameters

| Name | In   | Type         | Required | Description         |
| ---- | ---- | ------------ | -------- | ------------------- |
| `id` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "last_seen_at": "2019-08-24T14:15:22Z",
    "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
    "roles": [
      {
        "display_name": "string",
        "name": "string"
      }
    ],
    "status": "active",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Cancel template version by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templateversions/{id}/cancel \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templateversions/{id}/cancel`

### Parameters

| Name | In   | Type         | Required | Description         |
| ---- | ---- | ------------ | -------- | ------------------- |
| `id` | path | string(uuid) | true     | Template version ID |

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

## Create template version dry-run

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templateversions/{id}/dry-run \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /templateversions/{id}/dry-run`

> Body parameter

```json
{
  "parameter_values": [
    {
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
  "workspace_name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                 | Required | Description         |
| ------ | ---- | ---------------------------------------------------------------------------------------------------- | -------- | ------------------- |
| `id`   | path | string(uuid)                                                                                         | true     | Template version ID |
| `body` | body | [codersdk.CreateTemplateVersionDryRunRequest](schemas.md#codersdkcreatetemplateversiondryrunrequest) | true     | Dry-run request     |

### Example responses

> 201 Response

```json
{
  "canceled_at": "2019-08-24T14:15:22Z",
  "completed_at": "2019-08-24T14:15:22Z",
  "created_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "started_at": "2019-08-24T14:15:22Z",
  "status": "pending",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                       |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template version logs by template version ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{id}/resources \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{id}/resources`

### Parameters

| Name     | In    | Type         | Required | Description           |
| -------- | ----- | ------------ | -------- | --------------------- |
| `id`     | path  | string(uuid) | true     | Template version ID   |
| `before` | query | integer      | false    | Before Unix timestamp |
| `after`  | query | integer      | false    | After Unix timestamp  |
| `follow` | query | boolean      | false    | Follow log stream     |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "log_level": "trace",
    "log_source": "string",
    "output": "string",
    "stage": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerJobLog](schemas.md#codersdkprovisionerjoblog) |

<h3 id="get-template-version-logs-by-template-version-id-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type    | Required | Restrictions | Description |
| -------------- | ------- | -------- | ------------ | ----------- |
| `[array item]` | array   | false    |              |             |
| `» created_at` | string  | false    |              |             |
| `» id`         | integer | false    |              |             |
| `» log_level`  | string  | false    |              |             |
| `» log_source` | string  | false    |              |             |
| `» output`     | string  | false    |              |             |
| `» stage`      | string  | false    |              |             |

#### Enumerated Values

| Property    | Value   |
| ----------- | ------- |
| `log_level` | `trace` |
| `log_level` | `debug` |
| `log_level` | `info`  |
| `log_level` | `warn`  |
| `log_level` | `error` |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template version schema by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{id}/schema \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{id}/schema`

### Parameters

| Name | In   | Type         | Required | Description         |
| ---- | ---- | ------------ | -------- | ------------------- |
| `id` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "string",
    "default_source_value": true,
    "destination_scheme": "string",
    "id": "string",
    "name": "string",
    "schema_id": "string",
    "scope": "string",
    "scope_id": "string",
    "source_scheme": "string",
    "source_value": "string",
    "updated_at": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [parameter.ComputedValue](schemas.md#parametercomputedvalue) |

<h3 id="get-template-version-schema-by-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                     | Type    | Required | Restrictions | Description |
| ------------------------ | ------- | -------- | ------------ | ----------- |
| `[array item]`           | array   | false    |              |             |
| `» created_at`           | string  | false    |              |             |
| `» default_source_value` | boolean | false    |              |             |
| `» destination_scheme`   | string  | false    |              |             |
| `» id`                   | string  | false    |              |             |
| `» name`                 | string  | false    |              |             |
| `» schema_id`            | string  | false    |              |             |
| `» scope`                | string  | false    |              |             |
| `» scope_id`             | string  | false    |              |             |
| `» source_scheme`        | string  | false    |              |             |
| `» source_value`         | string  | false    |              |             |
| `» updated_at`           | string  | false    |              |             |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template version dry-run by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversionid}/dry-run/{jobid} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversionid}/dry-run/{jobid}`

### Parameters

| Name                | In   | Type         | Required | Description         |
| ------------------- | ---- | ------------ | -------- | ------------------- |
| `templateversionid` | path | string(uuid) | true     | Template version ID |
| `jobid`             | path | string(uuid) | true     | Job ID              |

### Example responses

> 200 Response

```json
{
  "canceled_at": "2019-08-24T14:15:22Z",
  "completed_at": "2019-08-24T14:15:22Z",
  "created_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "started_at": "2019-08-24T14:15:22Z",
  "status": "pending",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Cancel template version dry-run by job ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templateversions/{templateversionid}/dry-run/{jobid}/cancel \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templateversions/{templateversionid}/dry-run/{jobid}/cancel`

### Parameters

| Name | In   | Type         | Required | Description         |
| ---- | ---- | ------------ | -------- | ------------------- |
| `id` | path | string(uuid) | true     | Template version ID |

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

## Get template version dry-run logs by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversionid}/dry-run/{jobid}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversionid}/dry-run/{jobid}/logs`

### Parameters

| Name                | In    | Type         | Required | Description           |
| ------------------- | ----- | ------------ | -------- | --------------------- |
| `templateversionid` | path  | string(uuid) | true     | Template version ID   |
| `jobid`             | path  | string(uuid) | true     | Job ID                |
| `before`            | query | integer      | false    | Before Unix timestamp |
| `after`             | query | integer      | false    | After Unix timestamp  |
| `follow`            | query | boolean      | false    | Follow log stream     |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "log_level": "trace",
    "log_source": "string",
    "output": "string",
    "stage": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerJobLog](schemas.md#codersdkprovisionerjoblog) |

<h3 id="get-template-version-dry-run-logs-by-job-id-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type    | Required | Restrictions | Description |
| -------------- | ------- | -------- | ------------ | ----------- |
| `[array item]` | array   | false    |              |             |
| `» created_at` | string  | false    |              |             |
| `» id`         | integer | false    |              |             |
| `» log_level`  | string  | false    |              |             |
| `» log_source` | string  | false    |              |             |
| `» output`     | string  | false    |              |             |
| `» stage`      | string  | false    |              |             |

#### Enumerated Values

| Property    | Value   |
| ----------- | ------- |
| `log_level` | `trace` |
| `log_level` | `debug` |
| `log_level` | `info`  |
| `log_level` | `warn`  |
| `log_level` | `error` |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template version dry-run resources by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversionid}/dry-run/{jobid}/resources \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversionid}/dry-run/{jobid}/resources`

### Parameters

| Name                | In   | Type         | Required | Description         |
| ------------------- | ---- | ------------ | -------- | ------------------- |
| `templateversionid` | path | string(uuid) | true     | Template version ID |
| `jobid`             | path | string(uuid) | true     | Job ID              |

### Example responses

> 200 Response

```json
[
  {
    "agents": [
      {
        "apps": [
          {
            "command": "string",
            "display_name": "string",
            "external": true,
            "health": "string",
            "healthcheck": {
              "interval": 0,
              "threshold": 0,
              "url": "string"
            },
            "icon": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "sharing_level": "owner",
            "slug": "string",
            "subdomain": true,
            "url": "string"
          }
        ],
        "architecture": "string",
        "connection_timeout_seconds": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "directory": "string",
        "disconnected_at": "2019-08-24T14:15:22Z",
        "environment_variables": {
          "property1": "string",
          "property2": "string"
        },
        "first_connected_at": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "instance_id": "string",
        "last_connected_at": "2019-08-24T14:15:22Z",
        "latency": {
          "property1": {
            "latency_ms": 0,
            "preferred": true
          },
          "property2": {
            "latency_ms": 0,
            "preferred": true
          }
        },
        "name": "string",
        "operating_system": "string",
        "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
        "startup_script": "string",
        "status": "connecting",
        "troubleshooting_url": "string",
        "updated_at": "2019-08-24T14:15:22Z",
        "version": "string"
      }
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "hide": true,
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
    "metadata": [
      {
        "key": "string",
        "sensitive": true,
        "value": "string"
      }
    ],
    "name": "string",
    "type": "string",
    "workspace_transition": "start"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceResource](schemas.md#codersdkworkspaceresource) |

<h3 id="get-template-version-dry-run-resources-by-job-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                            | Type                   | Required | Restrictions | Description                                                                                                                                                                                                                                             |
| ------------------------------- | ---------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                  | array                  | false    |              |                                                                                                                                                                                                                                                         |
| `» agents`                      | array                  | false    |              |                                                                                                                                                                                                                                                         |
| `»» apps`                       | array                  | false    |              |                                                                                                                                                                                                                                                         |
| `»»» command`                   | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»» display_name`              | string                 | false    |              | DisplayName is a friendly name for the app.                                                                                                                                                                                                             |
| `»»» external`                  | boolean                | false    |              | External specifies whether the URL should be opened externally on<br>the client or not.                                                                                                                                                                 |
| `»»» health`                    | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»» healthcheck`               | `codersdk.Healthcheck` | false    |              |                                                                                                                                                                                                                                                         |
| `»»»» interval`                 | integer                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                               |
| `»»»» threshold`                | integer                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                                        |
| `»»»» url`                      | string                 | false    |              | URL specifies the endpoint to check for the app health.                                                                                                                                                                                                 |
| `»»» icon`                      | string                 | false    |              | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                     |
| `»»» id`                        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»» sharing_level`             | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»» slug`                      | string                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                           |
| `»»» subdomain`                 | boolean                | false    |              | Subdomain denotes whether the app should be accessed via a path on the<br>`coder server` or via a hostname-based dev URL. If this is set to true<br>and there is no app wildcard configured on the server, the app will not<br>be accessible in the UI. |
| `»»» url`                       | string                 | false    |              | URL is the address being proxied to inside the workspace.<br>If external is specified, this will be opened on the client.                                                                                                                               |
| `»» architecture`               | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» connection_timeout_seconds` | integer                | false    |              |                                                                                                                                                                                                                                                         |
| `»» created_at`                 | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» directory`                  | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» disconnected_at`            | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» environment_variables`      | object                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»» [any property]`            | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» first_connected_at`         | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» id`                         | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» instance_id`                | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» last_connected_at`          | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» latency`                    | object                 | false    |              | DERPLatency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                                 |
| `»»» [any property]`            | `codersdk.DERPRegion`  | false    |              |                                                                                                                                                                                                                                                         |
| `»»»» latency_ms`               | number                 | false    |              |                                                                                                                                                                                                                                                         |
| `»»»» preferred`                | boolean                | false    |              |                                                                                                                                                                                                                                                         |
| `»» name`                       | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» operating_system`           | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» resource_id`                | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» startup_script`             | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» status`                     | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» troubleshooting_url`        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» updated_at`                 | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» version`                    | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» created_at`                  | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» daily_cost`                  | integer                | false    |              |                                                                                                                                                                                                                                                         |
| `» hide`                        | boolean                | false    |              |                                                                                                                                                                                                                                                         |
| `» icon`                        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» id`                          | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» job_id`                      | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» metadata`                    | array                  | false    |              |                                                                                                                                                                                                                                                         |
| `»» key`                        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `»» sensitive`                  | boolean                | false    |              |                                                                                                                                                                                                                                                         |
| `»» value`                      | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» name`                        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» type`                        | string                 | false    |              |                                                                                                                                                                                                                                                         |
| `» workspace_transition`        | string                 | false    |              |                                                                                                                                                                                                                                                         |

#### Enumerated Values

| Property               | Value           |
| ---------------------- | --------------- |
| `sharing_level`        | `owner`         |
| `sharing_level`        | `authenticated` |
| `sharing_level`        | `public`        |
| `status`               | `connecting`    |
| `status`               | `connected`     |
| `status`               | `disconnected`  |
| `status`               | `timeout`       |
| `workspace_transition` | `start`         |
| `workspace_transition` | `stop`          |
| `workspace_transition` | `delete`        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
