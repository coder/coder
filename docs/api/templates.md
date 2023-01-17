# Templates

## Create group for organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/groups \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/groups`

> Body parameter

```json
{
  "avatar_url": "string",
  "name": "string",
  "quota_allowance": 0
}
```

### Parameters

| Name           | In   | Type                                                                 | Required | Description          |
| -------------- | ---- | -------------------------------------------------------------------- | -------- | -------------------- |
| `organization` | path | string                                                               | true     | Organization ID      |
| `body`         | body | [codersdk.CreateGroupRequest](schemas.md#codersdkcreategrouprequest) | true     | Create group request |

### Example responses

> 201 Response

```json
{
  "avatar_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
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
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                     |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
    "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
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
    "provisioner": "terraform",
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

| Name                                 | Type                                                                         | Required | Restrictions | Description                                  |
| ------------------------------------ | ---------------------------------------------------------------------------- | -------- | ------------ | -------------------------------------------- |
| `[array item]`                       | array                                                                        | false    |              |                                              |
| `» active_user_count`                | integer                                                                      | false    |              | Active user count is set to -1 when loading. |
| `» active_version_id`                | string(uuid)                                                                 | false    |              |                                              |
| `» allow_user_cancel_workspace_jobs` | boolean                                                                      | false    |              |                                              |
| `» build_time_stats`                 | [codersdk.TemplateBuildTimeStats](schemas.md#codersdktemplatebuildtimestats) | false    |              |                                              |
| `»» [any property]`                  | [codersdk.TransitionStats](schemas.md#codersdktransitionstats)               | false    |              |                                              |
| `»»» p50`                            | integer                                                                      | false    |              |                                              |
| `»»» p95`                            | integer                                                                      | false    |              |                                              |
| `» created_at`                       | string(date-time)                                                            | false    |              |                                              |
| `» created_by_id`                    | string(uuid)                                                                 | false    |              |                                              |
| `» created_by_name`                  | string                                                                       | false    |              |                                              |
| `» default_ttl_ms`                   | integer                                                                      | false    |              |                                              |
| `» description`                      | string                                                                       | false    |              |                                              |
| `» display_name`                     | string                                                                       | false    |              |                                              |
| `» icon`                             | string                                                                       | false    |              |                                              |
| `» id`                               | string(uuid)                                                                 | false    |              |                                              |
| `» name`                             | string                                                                       | false    |              |                                              |
| `» organization_id`                  | string(uuid)                                                                 | false    |              |                                              |
| `» provisioner`                      | string                                                                       | false    |              |                                              |
| `» updated_at`                       | string(date-time)                                                            | false    |              |                                              |
| `» workspace_owner_count`            | integer                                                                      | false    |              |                                              |

#### Enumerated Values

| Property      | Value       |
| ------------- | ----------- |
| `provisioner` | `terraform` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create template by organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/templates \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/templates`

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
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1"
}
```

### Parameters

| Name           | In   | Type                                                                       | Required | Description     |
| -------------- | ---- | -------------------------------------------------------------------------- | -------- | --------------- |
| `organization` | path | string                                                                     | true     | Organization ID |
| `body`         | body | [codersdk.CreateTemplateRequest](schemas.md#codersdkcreatetemplaterequest) | true     | Request body    |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
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
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template examples by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates/examples \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates/examples`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "description": "string",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "markdown": "string",
    "name": "string",
    "tags": ["string"],
    "url": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateExample](schemas.md#codersdktemplateexample) |

<h3 id="get-template-examples-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name            | Type         | Required | Restrictions | Description |
| --------------- | ------------ | -------- | ------------ | ----------- |
| `[array item]`  | array        | false    |              |             |
| `» description` | string       | false    |              |             |
| `» icon`        | string       | false    |              |             |
| `» id`          | string(uuid) | false    |              |             |
| `» markdown`    | string       | false    |              |             |
| `» name`        | string       | false    |              |             |
| `» tags`        | array        | false    |              |             |
| `» url`         | string       | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get templates by organization and template name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates/{templatename} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates/{templatename}`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |
| `templatename` | path | string       | true     | Template name   |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
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
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create template version by organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/templateversions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/templateversions`

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

| Name           | In   | Type                                                                                                 | Required | Description                     |
| -------------- | ---- | ---------------------------------------------------------------------------------------------------- | -------- | ------------------------------- |
| `organization` | path | string(uuid)                                                                                         | true     | Organization ID                 |
| `body`         | body | [codersdk.CreateTemplateVersionDryRunRequest](schemas.md#codersdkcreatetemplateversiondryrunrequest) | true     | Create template version request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "user@example.com",
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

| Status | Meaning                                                      | Description | Schema                                                         |
| ------ | ------------------------------------------------------------ | ----------- | -------------------------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version by organization and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templateversions/{templateversionname} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templateversions/{templateversionname}`

### Parameters

| Name                  | In   | Type         | Required | Description           |
| --------------------- | ---- | ------------ | -------- | --------------------- |
| `organization`        | path | string(uuid) | true     | Organization ID       |
| `templateversionname` | path | string       | true     | Template version name |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "user@example.com",
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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get previous template version by organization and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templateversions/{templateversionname}/previous \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templateversions/{templateversionname}/previous`

### Parameters

| Name                  | In   | Type         | Required | Description           |
| --------------------- | ---- | ------------ | -------- | --------------------- |
| `organization`        | path | string(uuid) | true     | Organization ID       |
| `templateversionname` | path | string       | true     | Template version name |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "user@example.com",
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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template metadata by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}`

### Parameters

| Name       | In   | Type         | Required | Description |
| ---------- | ---- | ------------ | -------- | ----------- |
| `template` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
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
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete template by ID

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/templates/{template} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /templates/{template}`

### Parameters

| Name       | In   | Type         | Required | Description |
| ---------- | ---- | ------------ | -------- | ----------- |
| `template` | path | string(uuid) | true     | Template ID |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update template metadata by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templates/{template} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templates/{template}`

### Parameters

| Name       | In   | Type         | Required | Description |
| ---------- | ---- | ------------ | -------- | ----------- |
| `template` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
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
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_owner_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template DAUs by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template}/daus \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}/daus`

### Parameters

| Name       | In   | Type         | Required | Description |
| ---------- | ---- | ------------ | -------- | ----------- |
| `template` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateDAUsResponse](schemas.md#codersdktemplatedausresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List template versions by template ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template}/versions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}/versions`

### Parameters

| Name       | In    | Type         | Required | Description |
| ---------- | ----- | ------------ | -------- | ----------- |
| `template` | path  | string(uuid) | true     | Template ID |
| `after_id` | query | string(uuid) | false    | After ID    |
| `limit`    | query | integer      | false    | Page limit  |
| `offset`   | query | integer      | false    | Page offset |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
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

| Name                  | Type                                                                     | Required | Restrictions | Description |
| --------------------- | ------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`        | array                                                                    | false    |              |             |
| `» created_at`        | string(date-time)                                                        | false    |              |             |
| `» created_by`        | [codersdk.User](schemas.md#codersdkuser)                                 | false    |              |             |
| `»» avatar_url`       | string(uri)                                                              | false    |              |             |
| `»» created_at`       | string(date-time)                                                        | true     |              |             |
| `»» email`            | string(email)                                                            | true     |              |             |
| `»» id`               | string(uuid)                                                             | true     |              |             |
| `»» last_seen_at`     | string(date-time)                                                        | false    |              |             |
| `»» organization_ids` | array                                                                    | false    |              |             |
| `»» roles`            | array                                                                    | false    |              |             |
| `»»» display_name`    | string                                                                   | false    |              |             |
| `»»» name`            | string                                                                   | false    |              |             |
| `»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)                     | false    |              |             |
| `»» username`         | string                                                                   | true     |              |             |
| `» id`                | string(uuid)                                                             | false    |              |             |
| `» job`               | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)             | false    |              |             |
| `»» canceled_at`      | string(date-time)                                                        | false    |              |             |
| `»» completed_at`     | string(date-time)                                                        | false    |              |             |
| `»» created_at`       | string(date-time)                                                        | false    |              |             |
| `»» error`            | string                                                                   | false    |              |             |
| `»» file_id`          | string(uuid)                                                             | false    |              |             |
| `»» id`               | string(uuid)                                                             | false    |              |             |
| `»» started_at`       | string(date-time)                                                        | false    |              |             |
| `»» status`           | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus) | false    |              |             |
| `»» tags`             | object                                                                   | false    |              |             |
| `»»» [any property]`  | string                                                                   | false    |              |             |
| `»» worker_id`        | string(uuid)                                                             | false    |              |             |
| `» name`              | string                                                                   | false    |              |             |
| `» organization_id`   | string(uuid)                                                             | false    |              |             |
| `» readme`            | string                                                                   | false    |              |             |
| `» template_id`       | string(uuid)                                                             | false    |              |             |
| `» updated_at`        | string(date-time)                                                        | false    |              |             |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update active template version by template ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templates/{template}/versions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templates/{template}/versions`

> Body parameter

```json
{
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
}
```

### Parameters

| Name       | In   | Type                                                                                   | Required | Description               |
| ---------- | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------- |
| `template` | path | string(uuid)                                                                           | true     | Template ID               |
| `body`     | body | [codersdk.UpdateActiveTemplateVersion](schemas.md#codersdkupdateactivetemplateversion) | true     | Modified template version |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version by template ID and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template}/versions/{templateversionname} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}/versions/{templateversionname}`

### Parameters

| Name                  | In   | Type         | Required | Description           |
| --------------------- | ---- | ------------ | -------- | --------------------- |
| `template`            | path | string(uuid) | true     | Template ID           |
| `templateversionname` | path | string       | true     | Template version name |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
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

| Name                  | Type                                                                     | Required | Restrictions | Description |
| --------------------- | ------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`        | array                                                                    | false    |              |             |
| `» created_at`        | string(date-time)                                                        | false    |              |             |
| `» created_by`        | [codersdk.User](schemas.md#codersdkuser)                                 | false    |              |             |
| `»» avatar_url`       | string(uri)                                                              | false    |              |             |
| `»» created_at`       | string(date-time)                                                        | true     |              |             |
| `»» email`            | string(email)                                                            | true     |              |             |
| `»» id`               | string(uuid)                                                             | true     |              |             |
| `»» last_seen_at`     | string(date-time)                                                        | false    |              |             |
| `»» organization_ids` | array                                                                    | false    |              |             |
| `»» roles`            | array                                                                    | false    |              |             |
| `»»» display_name`    | string                                                                   | false    |              |             |
| `»»» name`            | string                                                                   | false    |              |             |
| `»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)                     | false    |              |             |
| `»» username`         | string                                                                   | true     |              |             |
| `» id`                | string(uuid)                                                             | false    |              |             |
| `» job`               | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)             | false    |              |             |
| `»» canceled_at`      | string(date-time)                                                        | false    |              |             |
| `»» completed_at`     | string(date-time)                                                        | false    |              |             |
| `»» created_at`       | string(date-time)                                                        | false    |              |             |
| `»» error`            | string                                                                   | false    |              |             |
| `»» file_id`          | string(uuid)                                                             | false    |              |             |
| `»» id`               | string(uuid)                                                             | false    |              |             |
| `»» started_at`       | string(date-time)                                                        | false    |              |             |
| `»» status`           | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus) | false    |              |             |
| `»» tags`             | object                                                                   | false    |              |             |
| `»»» [any property]`  | string                                                                   | false    |              |             |
| `»» worker_id`        | string(uuid)                                                             | false    |              |             |
| `» name`              | string                                                                   | false    |              |             |
| `» organization_id`   | string(uuid)                                                             | false    |              |             |
| `» readme`            | string                                                                   | false    |              |             |
| `» template_id`       | string(uuid)                                                             | false    |              |             |
| `» updated_at`        | string(date-time)                                                        | false    |              |             |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "user@example.com",
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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Cancel template version by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templateversions/{templateversion}/cancel \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templateversions/{templateversion}/cancel`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create template version dry-run

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templateversions/{templateversion}/dry-run \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /templateversions/{templateversion}/dry-run`

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

| Name              | In   | Type                                                                                                 | Required | Description         |
| ----------------- | ---- | ---------------------------------------------------------------------------------------------------- | -------- | ------------------- |
| `templateversion` | path | string(uuid)                                                                                         | true     | Template version ID |
| `body`            | body | [codersdk.CreateTemplateVersionDryRunRequest](schemas.md#codersdkcreatetemplateversiondryrunrequest) | true     | Dry-run request     |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version dry-run by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/dry-run/{jobID} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/dry-run/{jobID}`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |
| `jobID`           | path | string(uuid) | true     | Job ID              |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Cancel template version dry-run by job ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templateversions/{templateversion}/dry-run/{jobID}/cancel \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templateversions/{templateversion}/dry-run/{jobID}/cancel`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `jobID`           | path | string(uuid) | true     | Job ID              |
| `templateversion` | path | string(uuid) | true     | Template version ID |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version dry-run logs by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/dry-run/{jobID}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/dry-run/{jobID}/logs`

### Parameters

| Name              | In    | Type         | Required | Description           |
| ----------------- | ----- | ------------ | -------- | --------------------- |
| `templateversion` | path  | string(uuid) | true     | Template version ID   |
| `jobID`           | path  | string(uuid) | true     | Job ID                |
| `before`          | query | integer      | false    | Before Unix timestamp |
| `after`           | query | integer      | false    | After Unix timestamp  |
| `follow`          | query | boolean      | false    | Follow log stream     |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "log_level": "trace",
    "log_source": "provisioner_daemon",
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

| Name           | Type                                               | Required | Restrictions | Description |
| -------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]` | array                                              | false    |              |             |
| `» created_at` | string(date-time)                                  | false    |              |             |
| `» id`         | integer                                            | false    |              |             |
| `» log_level`  | [codersdk.LogLevel](schemas.md#codersdkloglevel)   | false    |              |             |
| `» log_source` | [codersdk.LogSource](schemas.md#codersdklogsource) | false    |              |             |
| `» output`     | string                                             | false    |              |             |
| `» stage`      | string                                             | false    |              |             |

#### Enumerated Values

| Property     | Value                |
| ------------ | -------------------- |
| `log_level`  | `trace`              |
| `log_level`  | `debug`              |
| `log_level`  | `info`               |
| `log_level`  | `warn`               |
| `log_level`  | `error`              |
| `log_source` | `provisioner_daemon` |
| `log_source` | `provisioner`        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version dry-run resources by job ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/dry-run/{jobID}/resources \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/dry-run/{jobID}/resources`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |
| `jobID`           | path | string(uuid) | true     | Job ID              |

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
            "health": "disabled",
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

| Name                            | Type                                                                             | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------- | -------------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                  | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `» agents`                      | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»» apps`                       | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»»» command`                   | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                           | false    |              | »»display name is a friendly name for the app.                                                                                                                                                                                                 |
| `»»» external`                  | boolean                                                                          | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»» health`                    | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)             | false    |              |                                                                                                                                                                                                                                                |
| `»»» healthcheck`               | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                           | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»» interval`                 | integer                                                                          | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»» threshold`                | integer                                                                          | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»» url`                      | string                                                                           | false    |              | »»»url specifies the endpoint to check for the app health.                                                                                                                                                                                     |
| `»»» icon`                      | string                                                                           | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                        | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel) | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                      | string                                                                           | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» subdomain`                 | boolean                                                                          | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»» url`                       | string                                                                           | false    |              | »»url is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                       |
| `»» architecture`               | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» connection_timeout_seconds` | integer                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                 | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» directory`                  | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» disconnected_at`            | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» environment_variables`      | object                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`            | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» first_connected_at`         | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                         | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»» instance_id`                | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» last_connected_at`          | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» latency`                    | object                                                                           | false    |              | »latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                           |
| `»»» [any property]`            | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                             | false    |              |                                                                                                                                                                                                                                                |
| `»»»» latency_ms`               | number                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» preferred`                | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                       | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» operating_system`           | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» resource_id`                | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script`             | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» status`                     | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)         | false    |              |                                                                                                                                                                                                                                                |
| `»» troubleshooting_url`        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» updated_at`                 | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» version`                    | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                  | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                  | integer                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `» hide`                        | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `» icon`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» id`                          | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `» job_id`                      | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `» metadata`                    | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»» key`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» sensitive`                  | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» value`                      | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» name`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» type`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_transition`        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)           | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property               | Value           |
| ---------------------- | --------------- |
| `health`               | `disabled`      |
| `health`               | `initializing`  |
| `health`               | `healthy`       |
| `health`               | `unhealthy`     |
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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get logs by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/logs`

### Parameters

| Name              | In    | Type         | Required | Description           |
| ----------------- | ----- | ------------ | -------- | --------------------- |
| `templateversion` | path  | string(uuid) | true     | Template version ID   |
| `before`          | query | integer      | false    | Before Unix timestamp |
| `after`           | query | integer      | false    | After Unix timestamp  |
| `follow`          | query | boolean      | false    | Follow log stream     |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "log_level": "trace",
    "log_source": "provisioner_daemon",
    "output": "string",
    "stage": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerJobLog](schemas.md#codersdkprovisionerjoblog) |

<h3 id="get-logs-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type                                               | Required | Restrictions | Description |
| -------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]` | array                                              | false    |              |             |
| `» created_at` | string(date-time)                                  | false    |              |             |
| `» id`         | integer                                            | false    |              |             |
| `» log_level`  | [codersdk.LogLevel](schemas.md#codersdkloglevel)   | false    |              |             |
| `» log_source` | [codersdk.LogSource](schemas.md#codersdklogsource) | false    |              |             |
| `» output`     | string                                             | false    |              |             |
| `» stage`      | string                                             | false    |              |             |

#### Enumerated Values

| Property     | Value                |
| ------------ | -------------------- |
| `log_level`  | `trace`              |
| `log_level`  | `debug`              |
| `log_level`  | `info`               |
| `log_level`  | `warn`               |
| `log_level`  | `error`              |
| `log_source` | `provisioner_daemon` |
| `log_source` | `provisioner`        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get parameters by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/parameters \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/parameters`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "string",
    "default_source_value": true,
    "destination_scheme": "none",
    "id": "string",
    "name": "string",
    "schema_id": "string",
    "scope": "template",
    "scope_id": "string",
    "source_scheme": "none",
    "source_value": "string",
    "updated_at": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [parameter.ComputedValue](schemas.md#parametercomputedvalue) |

<h3 id="get-parameters-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                     | Type                                                                                 | Required | Restrictions | Description |
| ------------------------ | ------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`           | array                                                                                | false    |              |             |
| `» created_at`           | string                                                                               | false    |              |             |
| `» default_source_value` | boolean                                                                              | false    |              |             |
| `» destination_scheme`   | [database.ParameterDestinationScheme](schemas.md#databaseparameterdestinationscheme) | false    |              |             |
| `» id`                   | string                                                                               | false    |              |             |
| `» name`                 | string                                                                               | false    |              |             |
| `» schema_id`            | string                                                                               | false    |              |             |
| `» scope`                | [database.ParameterScope](schemas.md#databaseparameterscope)                         | false    |              |             |
| `» scope_id`             | string                                                                               | false    |              |             |
| `» source_scheme`        | [database.ParameterSourceScheme](schemas.md#databaseparametersourcescheme)           | false    |              |             |
| `» source_value`         | string                                                                               | false    |              |             |
| `» updated_at`           | string                                                                               | false    |              |             |

#### Enumerated Values

| Property             | Value                  |
| -------------------- | ---------------------- |
| `destination_scheme` | `none`                 |
| `destination_scheme` | `environment_variable` |
| `destination_scheme` | `provisioner_variable` |
| `scope`              | `template`             |
| `scope`              | `import_job`           |
| `scope`              | `workspace`            |
| `source_scheme`      | `none`                 |
| `source_scheme`      | `data`                 |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get resources by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/resources \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/resources`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

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
            "health": "disabled",
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

<h3 id="get-resources-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                            | Type                                                                             | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------- | -------------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                  | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `» agents`                      | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»» apps`                       | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»»» command`                   | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                           | false    |              | »»display name is a friendly name for the app.                                                                                                                                                                                                 |
| `»»» external`                  | boolean                                                                          | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»» health`                    | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)             | false    |              |                                                                                                                                                                                                                                                |
| `»»» healthcheck`               | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                           | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»» interval`                 | integer                                                                          | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»» threshold`                | integer                                                                          | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»» url`                      | string                                                                           | false    |              | »»»url specifies the endpoint to check for the app health.                                                                                                                                                                                     |
| `»»» icon`                      | string                                                                           | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                        | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel) | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                      | string                                                                           | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» subdomain`                 | boolean                                                                          | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»» url`                       | string                                                                           | false    |              | »»url is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                       |
| `»» architecture`               | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» connection_timeout_seconds` | integer                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                 | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» directory`                  | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» disconnected_at`            | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» environment_variables`      | object                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`            | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» first_connected_at`         | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                         | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»» instance_id`                | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» last_connected_at`          | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» latency`                    | object                                                                           | false    |              | »latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                           |
| `»»» [any property]`            | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                             | false    |              |                                                                                                                                                                                                                                                |
| `»»»» latency_ms`               | number                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» preferred`                | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                       | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» operating_system`           | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» resource_id`                | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script`             | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» status`                     | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)         | false    |              |                                                                                                                                                                                                                                                |
| `»» troubleshooting_url`        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» updated_at`                 | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» version`                    | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                  | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                  | integer                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `» hide`                        | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `» icon`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» id`                          | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `» job_id`                      | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                |
| `» metadata`                    | array                                                                            | false    |              |                                                                                                                                                                                                                                                |
| `»» key`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» sensitive`                  | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                |
| `»» value`                      | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» name`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» type`                        | string                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_transition`        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)           | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property               | Value           |
| ---------------------- | --------------- |
| `health`               | `disabled`      |
| `health`               | `initializing`  |
| `health`               | `healthy`       |
| `health`               | `unhealthy`     |
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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get rich parameters by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/rich-parameters \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/rich-parameters`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "string",
    "default_source_value": true,
    "destination_scheme": "none",
    "id": "string",
    "name": "string",
    "schema_id": "string",
    "scope": "template",
    "scope_id": "string",
    "source_scheme": "none",
    "source_value": "string",
    "updated_at": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [parameter.ComputedValue](schemas.md#parametercomputedvalue) |

<h3 id="get-rich-parameters-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                     | Type                                                                                 | Required | Restrictions | Description |
| ------------------------ | ------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`           | array                                                                                | false    |              |             |
| `» created_at`           | string                                                                               | false    |              |             |
| `» default_source_value` | boolean                                                                              | false    |              |             |
| `» destination_scheme`   | [database.ParameterDestinationScheme](schemas.md#databaseparameterdestinationscheme) | false    |              |             |
| `» id`                   | string                                                                               | false    |              |             |
| `» name`                 | string                                                                               | false    |              |             |
| `» schema_id`            | string                                                                               | false    |              |             |
| `» scope`                | [database.ParameterScope](schemas.md#databaseparameterscope)                         | false    |              |             |
| `» scope_id`             | string                                                                               | false    |              |             |
| `» source_scheme`        | [database.ParameterSourceScheme](schemas.md#databaseparametersourcescheme)           | false    |              |             |
| `» source_value`         | string                                                                               | false    |              |             |
| `» updated_at`           | string                                                                               | false    |              |             |

#### Enumerated Values

| Property             | Value                  |
| -------------------- | ---------------------- |
| `destination_scheme` | `none`                 |
| `destination_scheme` | `environment_variable` |
| `destination_scheme` | `provisioner_variable` |
| `scope`              | `template`             |
| `scope`              | `import_job`           |
| `scope`              | `workspace`            |
| `source_scheme`      | `none`                 |
| `source_scheme`      | `data`                 |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get schema by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/schema \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/schema`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "allow_override_destination": true,
    "allow_override_source": true,
    "created_at": "2019-08-24T14:15:22Z",
    "default_destination_scheme": "none",
    "default_refresh": "string",
    "default_source_scheme": "none",
    "default_source_value": "string",
    "description": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
    "name": "string",
    "redisplay_value": true,
    "validation_condition": "string",
    "validation_contains": ["string"],
    "validation_error": "string",
    "validation_type_system": "string",
    "validation_value_type": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ParameterSchema](schemas.md#codersdkparameterschema) |

<h3 id="get-schema-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                           | Type                                                                                 | Required | Restrictions | Description                                                                                                             |
| ------------------------------ | ------------------------------------------------------------------------------------ | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                 | array                                                                                | false    |              |                                                                                                                         |
| `» allow_override_destination` | boolean                                                                              | false    |              |                                                                                                                         |
| `» allow_override_source`      | boolean                                                                              | false    |              |                                                                                                                         |
| `» created_at`                 | string(date-time)                                                                    | false    |              |                                                                                                                         |
| `» default_destination_scheme` | [codersdk.ParameterDestinationScheme](schemas.md#codersdkparameterdestinationscheme) | false    |              |                                                                                                                         |
| `» default_refresh`            | string                                                                               | false    |              |                                                                                                                         |
| `» default_source_scheme`      | [codersdk.ParameterSourceScheme](schemas.md#codersdkparametersourcescheme)           | false    |              |                                                                                                                         |
| `» default_source_value`       | string                                                                               | false    |              |                                                                                                                         |
| `» description`                | string                                                                               | false    |              |                                                                                                                         |
| `» id`                         | string(uuid)                                                                         | false    |              |                                                                                                                         |
| `» job_id`                     | string(uuid)                                                                         | false    |              |                                                                                                                         |
| `» name`                       | string                                                                               | false    |              |                                                                                                                         |
| `» redisplay_value`            | boolean                                                                              | false    |              |                                                                                                                         |
| `» validation_condition`       | string                                                                               | false    |              |                                                                                                                         |
| `» validation_contains`        | array                                                                                | false    |              | This is a special array of items provided if the validation condition explicitly states the value must be one of a set. |
| `» validation_error`           | string                                                                               | false    |              |                                                                                                                         |
| `» validation_type_system`     | string                                                                               | false    |              |                                                                                                                         |
| `» validation_value_type`      | string                                                                               | false    |              |                                                                                                                         |

#### Enumerated Values

| Property                     | Value                  |
| ---------------------------- | ---------------------- |
| `default_destination_scheme` | `none`                 |
| `default_destination_scheme` | `environment_variable` |
| `default_destination_scheme` | `provisioner_variable` |
| `default_source_scheme`      | `none`                 |
| `default_source_scheme`      | `data`                 |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
