# Templates

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
    "allow_user_autostart": true,
    "allow_user_autostop": true,
    "allow_user_cancel_workspace_jobs": true,
    "autostart_requirement": {
      "days_of_week": ["monday"]
    },
    "autostop_requirement": {
      "days_of_week": ["monday"],
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
    "created_at": "2019-08-24T14:15:22Z",
    "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
    "created_by_name": "string",
    "default_ttl_ms": 0,
    "deprecated": true,
    "deprecation_message": "string",
    "description": "string",
    "display_name": "string",
    "failure_ttl_ms": 0,
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "max_ttl_ms": 0,
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "provisioner": "terraform",
    "require_active_version": true,
    "time_til_dormant_autodelete_ms": 0,
    "time_til_dormant_ms": 0,
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                    |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Template](schemas.md#codersdktemplate) |

<h3 id="get-templates-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                                                                                  | Type                                                                                     | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                    |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                                                                        | array                                                                                    | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» active_user_count`                                                                 | integer                                                                                  | false    |              | Active user count is set to -1 when loading.                                                                                                                                                                                                                                                                   |
| `» active_version_id`                                                                 | string(uuid)                                                                             | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» allow_user_autostart`                                                              | boolean                                                                                  | false    |              | Allow user autostart and AllowUserAutostop are enterprise-only. Their values are only used if your license is entitled to use the advanced template scheduling feature.                                                                                                                                        |
| `» allow_user_autostop`                                                               | boolean                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» allow_user_cancel_workspace_jobs`                                                  | boolean                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» autostart_requirement`                                                             | [codersdk.TemplateAutostartRequirement](schemas.md#codersdktemplateautostartrequirement) | false    |              |                                                                                                                                                                                                                                                                                                                |
| `»» days_of_week`                                                                     | array                                                                                    | false    |              | Days of week is a list of days of the week in which autostart is allowed to happen. If no days are specified, autostart is not allowed.                                                                                                                                                                        |
| `» autostop_requirement`                                                              | [codersdk.TemplateAutostopRequirement](schemas.md#codersdktemplateautostoprequirement)   | false    |              | Autostop requirement and AutostartRequirement are enterprise features. Its value is only used if your license is entitled to use the advanced template scheduling feature.                                                                                                                                     |
| `»» days_of_week`                                                                     | array                                                                                    | false    |              | Days of week is a list of days of the week on which restarts are required. Restarts happen within the user's quiet hours (in their configured timezone). If no days are specified, restarts are not required. Weekdays cannot be specified twice.                                                              |
| Restarts will only happen on weekdays in this list on weeks which line up with Weeks. |
| `»» weeks`                                                                            | integer                                                                                  | false    |              | Weeks is the number of weeks between required restarts. Weeks are synced across all workspaces (and Coder deployments) using modulo math on a hardcoded epoch week of January 2nd, 2023 (the first Monday of 2023). Values of 0 or 1 indicate weekly restarts. Values of 2 indicate fortnightly restarts, etc. |
| `» build_time_stats`                                                                  | [codersdk.TemplateBuildTimeStats](schemas.md#codersdktemplatebuildtimestats)             | false    |              |                                                                                                                                                                                                                                                                                                                |
| `»» [any property]`                                                                   | [codersdk.TransitionStats](schemas.md#codersdktransitionstats)                           | false    |              |                                                                                                                                                                                                                                                                                                                |
| `»»» p50`                                                                             | integer                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `»»» p95`                                                                             | integer                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» created_at`                                                                        | string(date-time)                                                                        | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» created_by_id`                                                                     | string(uuid)                                                                             | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» created_by_name`                                                                   | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» default_ttl_ms`                                                                    | integer                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» deprecated`                                                                        | boolean                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» deprecation_message`                                                               | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» description`                                                                       | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» display_name`                                                                      | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» failure_ttl_ms`                                                                    | integer                                                                                  | false    |              | Failure ttl ms TimeTilDormantMillis, and TimeTilDormantAutoDeleteMillis are enterprise-only. Their values are used if your license is entitled to use the advanced template scheduling feature.                                                                                                                |
| `» icon`                                                                              | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» id`                                                                                | string(uuid)                                                                             | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» max_ttl_ms`                                                                        | integer                                                                                  | false    |              | Max ttl ms remove max_ttl once autostop_requirement is matured                                                                                                                                                                                                                                                 |
| `» name`                                                                              | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» organization_id`                                                                   | string(uuid)                                                                             | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» provisioner`                                                                       | string                                                                                   | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» require_active_version`                                                            | boolean                                                                                  | false    |              | Require active version mandates that workspaces are built with the active template version.                                                                                                                                                                                                                    |
| `» time_til_dormant_autodelete_ms`                                                    | integer                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» time_til_dormant_ms`                                                               | integer                                                                                  | false    |              |                                                                                                                                                                                                                                                                                                                |
| `» updated_at`                                                                        | string(date-time)                                                                        | false    |              |                                                                                                                                                                                                                                                                                                                |

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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": ["monday"]
  },
  "autostop_requirement": {
    "days_of_week": ["monday"],
    "weeks": 0
  },
  "default_ttl_ms": 0,
  "delete_ttl_ms": 0,
  "description": "string",
  "disable_everyone_group_access": true,
  "display_name": "string",
  "dormant_ttl_ms": 0,
  "failure_ttl_ms": 0,
  "icon": "string",
  "max_ttl_ms": 0,
  "name": "string",
  "require_active_version": true,
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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": ["monday"]
  },
  "autostop_requirement": {
    "days_of_week": ["monday"],
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "deprecated": true,
  "deprecation_message": "string",
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "max_ttl_ms": 0,
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "require_active_version": true,
  "time_til_dormant_autodelete_ms": 0,
  "time_til_dormant_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": ["monday"]
  },
  "autostop_requirement": {
    "days_of_week": ["monday"],
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "deprecated": true,
  "deprecation_message": "string",
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "max_ttl_ms": 0,
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "require_active_version": true,
  "time_til_dormant_autodelete_ms": 0,
  "time_til_dormant_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Template](schemas.md#codersdktemplate) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template version by organization, template, and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates/{templatename}/versions/{templateversionname} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates/{templatename}/versions/{templateversionname}`

### Parameters

| Name                  | In   | Type         | Required | Description           |
| --------------------- | ---- | ------------ | -------- | --------------------- |
| `organization`        | path | string(uuid) | true     | Organization ID       |
| `templatename`        | path | string       | true     | Template name         |
| `templateversionname` | path | string       | true     | Template version name |

### Example responses

> 200 Response

```json
{
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": ["UNSUPPORTED_WORKSPACES"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get previous template version by organization, template, and name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/templates/{templatename}/versions/{templateversionname}/previous \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/templates/{templatename}/versions/{templateversionname}/previous`

### Parameters

| Name                  | In   | Type         | Required | Description           |
| --------------------- | ---- | ------------ | -------- | --------------------- |
| `organization`        | path | string(uuid) | true     | Organization ID       |
| `templatename`        | path | string       | true     | Template name         |
| `templateversionname` | path | string       | true     | Template version name |

### Example responses

> 200 Response

```json
{
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": ["UNSUPPORTED_WORKSPACES"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

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
  "example_id": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "message": "string",
  "name": "string",
  "provisioner": "terraform",
  "storage_method": "file",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "user_variable_values": [
    {
      "name": "string",
      "value": "string"
    }
  ]
}
```

### Parameters

| Name           | In   | Type                                                                                     | Required | Description                     |
| -------------- | ---- | ---------------------------------------------------------------------------------------- | -------- | ------------------------------- |
| `organization` | path | string(uuid)                                                                             | true     | Organization ID                 |
| `body`         | body | [codersdk.CreateTemplateVersionRequest](schemas.md#codersdkcreatetemplateversionrequest) | true     | Create template version request |

### Example responses

> 201 Response

```json
{
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": ["UNSUPPORTED_WORKSPACES"]
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                         |
| ------ | ------------------------------------------------------------ | ----------- | -------------------------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": ["monday"]
  },
  "autostop_requirement": {
    "days_of_week": ["monday"],
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "deprecated": true,
  "deprecation_message": "string",
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "max_ttl_ms": 0,
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "require_active_version": true,
  "time_til_dormant_autodelete_ms": 0,
  "time_til_dormant_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": ["monday"]
  },
  "autostop_requirement": {
    "days_of_week": ["monday"],
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
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "deprecated": true,
  "deprecation_message": "string",
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "max_ttl_ms": 0,
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "require_active_version": true,
  "time_til_dormant_autodelete_ms": 0,
  "time_til_dormant_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
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
  ],
  "tz_hour_offset": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DAUsResponse](schemas.md#codersdkdausresponse) |

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

| Name               | In    | Type         | Required | Description                           |
| ------------------ | ----- | ------------ | -------- | ------------------------------------- |
| `template`         | path  | string(uuid) | true     | Template ID                           |
| `after_id`         | query | string(uuid) | false    | After ID                              |
| `include_archived` | query | boolean      | false    | Include archived versions in the list |
| `limit`            | query | integer      | false    | Page limit                            |
| `offset`           | query | integer      | false    | Page offset                           |

### Example responses

> 200 Response

```json
[
  {
    "archived": true,
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "username": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job": {
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "error_code": "REQUIRED_TEMPLATE_VARIABLES",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "queue_position": 0,
      "queue_size": 0,
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "message": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "readme": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "updated_at": "2019-08-24T14:15:22Z",
    "warnings": ["UNSUPPORTED_WORKSPACES"]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

<h3 id="list-template-versions-by-template-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                                     | Required | Restrictions | Description |
| -------------------- | ------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`       | array                                                                    | false    |              |             |
| `» archived`         | boolean                                                                  | false    |              |             |
| `» created_at`       | string(date-time)                                                        | false    |              |             |
| `» created_by`       | [codersdk.MinimalUser](schemas.md#codersdkminimaluser)                   | false    |              |             |
| `»» avatar_url`      | string(uri)                                                              | false    |              |             |
| `»» id`              | string(uuid)                                                             | true     |              |             |
| `»» username`        | string                                                                   | true     |              |             |
| `» id`               | string(uuid)                                                             | false    |              |             |
| `» job`              | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)             | false    |              |             |
| `»» canceled_at`     | string(date-time)                                                        | false    |              |             |
| `»» completed_at`    | string(date-time)                                                        | false    |              |             |
| `»» created_at`      | string(date-time)                                                        | false    |              |             |
| `»» error`           | string                                                                   | false    |              |             |
| `»» error_code`      | [codersdk.JobErrorCode](schemas.md#codersdkjoberrorcode)                 | false    |              |             |
| `»» file_id`         | string(uuid)                                                             | false    |              |             |
| `»» id`              | string(uuid)                                                             | false    |              |             |
| `»» queue_position`  | integer                                                                  | false    |              |             |
| `»» queue_size`      | integer                                                                  | false    |              |             |
| `»» started_at`      | string(date-time)                                                        | false    |              |             |
| `»» status`          | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus) | false    |              |             |
| `»» tags`            | object                                                                   | false    |              |             |
| `»»» [any property]` | string                                                                   | false    |              |             |
| `»» worker_id`       | string(uuid)                                                             | false    |              |             |
| `» message`          | string                                                                   | false    |              |             |
| `» name`             | string                                                                   | false    |              |             |
| `» organization_id`  | string(uuid)                                                             | false    |              |             |
| `» readme`           | string                                                                   | false    |              |             |
| `» template_id`      | string(uuid)                                                             | false    |              |             |
| `» updated_at`       | string(date-time)                                                        | false    |              |             |
| `» warnings`         | array                                                                    | false    |              |             |

#### Enumerated Values

| Property     | Value                         |
| ------------ | ----------------------------- |
| `error_code` | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`     | `pending`                     |
| `status`     | `running`                     |
| `status`     | `succeeded`                   |
| `status`     | `canceling`                   |
| `status`     | `canceled`                    |
| `status`     | `failed`                      |

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

## Archive template unused versions by template id

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templates/{template}/versions/archive \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /templates/{template}/versions/archive`

> Body parameter

```json
{
  "all": true
}
```

### Parameters

| Name       | In   | Type                                                                                         | Required | Description     |
| ---------- | ---- | -------------------------------------------------------------------------------------------- | -------- | --------------- |
| `template` | path | string(uuid)                                                                                 | true     | Template ID     |
| `body`     | body | [codersdk.ArchiveTemplateVersionsRequest](schemas.md#codersdkarchivetemplateversionsrequest) | true     | Archive request |

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
    "archived": true,
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": {
      "avatar_url": "http://example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "username": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "job": {
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "error_code": "REQUIRED_TEMPLATE_VARIABLES",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "queue_position": 0,
      "queue_size": 0,
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "message": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "readme": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "updated_at": "2019-08-24T14:15:22Z",
    "warnings": ["UNSUPPORTED_WORKSPACES"]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

<h3 id="get-template-version-by-template-id-and-name-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                                     | Required | Restrictions | Description |
| -------------------- | ------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`       | array                                                                    | false    |              |             |
| `» archived`         | boolean                                                                  | false    |              |             |
| `» created_at`       | string(date-time)                                                        | false    |              |             |
| `» created_by`       | [codersdk.MinimalUser](schemas.md#codersdkminimaluser)                   | false    |              |             |
| `»» avatar_url`      | string(uri)                                                              | false    |              |             |
| `»» id`              | string(uuid)                                                             | true     |              |             |
| `»» username`        | string                                                                   | true     |              |             |
| `» id`               | string(uuid)                                                             | false    |              |             |
| `» job`              | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)             | false    |              |             |
| `»» canceled_at`     | string(date-time)                                                        | false    |              |             |
| `»» completed_at`    | string(date-time)                                                        | false    |              |             |
| `»» created_at`      | string(date-time)                                                        | false    |              |             |
| `»» error`           | string                                                                   | false    |              |             |
| `»» error_code`      | [codersdk.JobErrorCode](schemas.md#codersdkjoberrorcode)                 | false    |              |             |
| `»» file_id`         | string(uuid)                                                             | false    |              |             |
| `»» id`              | string(uuid)                                                             | false    |              |             |
| `»» queue_position`  | integer                                                                  | false    |              |             |
| `»» queue_size`      | integer                                                                  | false    |              |             |
| `»» started_at`      | string(date-time)                                                        | false    |              |             |
| `»» status`          | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus) | false    |              |             |
| `»» tags`            | object                                                                   | false    |              |             |
| `»»» [any property]` | string                                                                   | false    |              |             |
| `»» worker_id`       | string(uuid)                                                             | false    |              |             |
| `» message`          | string                                                                   | false    |              |             |
| `» name`             | string                                                                   | false    |              |             |
| `» organization_id`  | string(uuid)                                                             | false    |              |             |
| `» readme`           | string                                                                   | false    |              |             |
| `» template_id`      | string(uuid)                                                             | false    |              |             |
| `» updated_at`       | string(date-time)                                                        | false    |              |             |
| `» warnings`         | array                                                                    | false    |              |             |

#### Enumerated Values

| Property     | Value                         |
| ------------ | ----------------------------- |
| `error_code` | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`     | `pending`                     |
| `status`     | `running`                     |
| `status`     | `succeeded`                   |
| `status`     | `canceling`                   |
| `status`     | `canceled`                    |
| `status`     | `failed`                      |

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
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": ["UNSUPPORTED_WORKSPACES"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Patch template version by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templateversions/{templateversion} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templateversions/{templateversion}`

> Body parameter

```json
{
  "message": "string",
  "name": "string"
}
```

### Parameters

| Name              | In   | Type                                                                                   | Required | Description                    |
| ----------------- | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------------ |
| `templateversion` | path | string(uuid)                                                                           | true     | Template version ID            |
| `body`            | body | [codersdk.PatchTemplateVersionRequest](schemas.md#codersdkpatchtemplateversionrequest) | true     | Patch template version request |

### Example responses

> 200 Response

```json
{
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": ["UNSUPPORTED_WORKSPACES"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateVersion](schemas.md#codersdktemplateversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Archive template version

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templateversions/{templateversion}/archive \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /templateversions/{templateversion}/archive`

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
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "user_variable_values": [
    {
      "name": "string",
      "value": "string"
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
  "error_code": "REQUIRED_TEMPLATE_VARIABLES",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "queue_position": 0,
  "queue_size": 0,
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
  "error_code": "REQUIRED_TEMPLATE_VARIABLES",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "queue_position": 0,
  "queue_size": 0,
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
        "api_version": "string",
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
            "subdomain_name": "string",
            "url": "string"
          }
        ],
        "architecture": "string",
        "connection_timeout_seconds": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "directory": "string",
        "disconnected_at": "2019-08-24T14:15:22Z",
        "display_apps": ["vscode"],
        "environment_variables": {
          "property1": "string",
          "property2": "string"
        },
        "expanded_directory": "string",
        "first_connected_at": "2019-08-24T14:15:22Z",
        "health": {
          "healthy": false,
          "reason": "agent has lost connection"
        },
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
        "lifecycle_state": "created",
        "log_sources": [
          {
            "created_at": "2019-08-24T14:15:22Z",
            "display_name": "string",
            "icon": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
          }
        ],
        "logs_length": 0,
        "logs_overflowed": true,
        "name": "string",
        "operating_system": "string",
        "ready_at": "2019-08-24T14:15:22Z",
        "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
        "scripts": [
          {
            "cron": "string",
            "log_path": "string",
            "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
            "run_on_start": true,
            "run_on_stop": true,
            "script": "string",
            "start_blocks_login": true,
            "timeout": 0
          }
        ],
        "started_at": "2019-08-24T14:15:22Z",
        "startup_script_behavior": "blocking",
        "status": "connecting",
        "subsystems": ["envbox"],
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

| Name                            | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------- | ------------------------------------------------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                  | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `» agents`                      | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» api_version`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» apps`                       | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» command`                   | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                                                 | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `»»» external`                  | boolean                                                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»» health`                    | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»» healthcheck`               | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                                                 | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»» interval`                 | integer                                                                                                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»» threshold`                | integer                                                                                                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»» url`                      | string                                                                                                 | false    |              | URL specifies the endpoint to check for the app health.                                                                                                                                                                                        |
| `»»» icon`                      | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                      | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» subdomain`                 | boolean                                                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»» subdomain_name`            | string                                                                                                 | false    |              | Subdomain name is the application domain exposed on the `coder server`.                                                                                                                                                                        |
| `»»» url`                       | string                                                                                                 | false    |              | URL is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |
| `»» architecture`               | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» connection_timeout_seconds` | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» directory`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» disconnected_at`            | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» display_apps`               | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» environment_variables`      | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» expanded_directory`         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» first_connected_at`         | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» health`                     | [codersdk.WorkspaceAgentHealth](schemas.md#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                                                                                        |
| `»»» healthy`                   | boolean                                                                                                | false    |              | Healthy is true if the agent is healthy.                                                                                                                                                                                                       |
| `»»» reason`                    | string                                                                                                 | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.                                                                                                                                                  |
| `»» id`                         | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» instance_id`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» last_connected_at`          | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» latency`                    | object                                                                                                 | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                            |
| `»»» [any property]`            | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» latency_ms`               | number                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» preferred`                | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» lifecycle_state`            | [codersdk.WorkspaceAgentLifecycle](schemas.md#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                                                                                                |
| `»» log_sources`                | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» created_at`                | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» icon`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» workspace_agent_id`        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» logs_length`                | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» logs_overflowed`            | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» operating_system`           | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» ready_at`                   | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» resource_id`                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» scripts`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» cron`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» log_path`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» log_source_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» run_on_start`              | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» run_on_stop`               | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» script`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» start_blocks_login`        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» timeout`                   | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» started_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script_behavior`    | [codersdk.WorkspaceAgentStartupScriptBehavior](schemas.md#codersdkworkspaceagentstartupscriptbehavior) | false    |              | Startup script behavior is a legacy field that is deprecated in favor of the `coder_script` resource. It's only referenced by old clients. Deprecated: Remove in the future!                                                                   |
| `»» status`                     | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»» subsystems`                 | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» troubleshooting_url`        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» updated_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» version`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                  | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                  | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» hide`                        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» icon`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» id`                          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» job_id`                      | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» metadata`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» key`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» sensitive`                  | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» value`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» name`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» type`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_transition`        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property                  | Value              |
| ------------------------- | ------------------ |
| `health`                  | `disabled`         |
| `health`                  | `initializing`     |
| `health`                  | `healthy`          |
| `health`                  | `unhealthy`        |
| `sharing_level`           | `owner`            |
| `sharing_level`           | `authenticated`    |
| `sharing_level`           | `public`           |
| `lifecycle_state`         | `created`          |
| `lifecycle_state`         | `starting`         |
| `lifecycle_state`         | `start_timeout`    |
| `lifecycle_state`         | `start_error`      |
| `lifecycle_state`         | `ready`            |
| `lifecycle_state`         | `shutting_down`    |
| `lifecycle_state`         | `shutdown_timeout` |
| `lifecycle_state`         | `shutdown_error`   |
| `lifecycle_state`         | `off`              |
| `startup_script_behavior` | `blocking`         |
| `startup_script_behavior` | `non-blocking`     |
| `status`                  | `connecting`       |
| `status`                  | `connected`        |
| `status`                  | `disconnected`     |
| `status`                  | `timeout`          |
| `workspace_transition`    | `start`            |
| `workspace_transition`    | `stop`             |
| `workspace_transition`    | `delete`           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get external auth by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/external-auth \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/external-auth`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "authenticate_url": "string",
    "authenticated": true,
    "display_icon": "string",
    "display_name": "string",
    "id": "string",
    "type": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                          |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersionExternalAuth](schemas.md#codersdktemplateversionexternalauth) |

<h3 id="get-external-auth-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type    | Required | Restrictions | Description |
| -------------------- | ------- | -------- | ------------ | ----------- |
| `[array item]`       | array   | false    |              |             |
| `» authenticate_url` | string  | false    |              |             |
| `» authenticated`    | boolean | false    |              |             |
| `» display_icon`     | string  | false    |              |             |
| `» display_name`     | string  | false    |              |             |
| `» id`               | string  | false    |              |             |
| `» type`             | string  | false    |              |             |

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

| Name              | In    | Type         | Required | Description         |
| ----------------- | ----- | ------------ | -------- | ------------------- |
| `templateversion` | path  | string(uuid) | true     | Template version ID |
| `before`          | query | integer      | false    | Before log id       |
| `after`           | query | integer      | false    | After log id        |
| `follow`          | query | boolean      | false    | Follow log stream   |

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

## Removed: Get parameters by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/parameters \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/parameters`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

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
        "api_version": "string",
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
            "subdomain_name": "string",
            "url": "string"
          }
        ],
        "architecture": "string",
        "connection_timeout_seconds": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "directory": "string",
        "disconnected_at": "2019-08-24T14:15:22Z",
        "display_apps": ["vscode"],
        "environment_variables": {
          "property1": "string",
          "property2": "string"
        },
        "expanded_directory": "string",
        "first_connected_at": "2019-08-24T14:15:22Z",
        "health": {
          "healthy": false,
          "reason": "agent has lost connection"
        },
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
        "lifecycle_state": "created",
        "log_sources": [
          {
            "created_at": "2019-08-24T14:15:22Z",
            "display_name": "string",
            "icon": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
          }
        ],
        "logs_length": 0,
        "logs_overflowed": true,
        "name": "string",
        "operating_system": "string",
        "ready_at": "2019-08-24T14:15:22Z",
        "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
        "scripts": [
          {
            "cron": "string",
            "log_path": "string",
            "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
            "run_on_start": true,
            "run_on_stop": true,
            "script": "string",
            "start_blocks_login": true,
            "timeout": 0
          }
        ],
        "started_at": "2019-08-24T14:15:22Z",
        "startup_script_behavior": "blocking",
        "status": "connecting",
        "subsystems": ["envbox"],
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

| Name                            | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------- | ------------------------------------------------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                  | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `» agents`                      | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» api_version`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» apps`                       | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» command`                   | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                                                 | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `»»» external`                  | boolean                                                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»» health`                    | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»» healthcheck`               | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                                                 | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»» interval`                 | integer                                                                                                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»» threshold`                | integer                                                                                                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»» url`                      | string                                                                                                 | false    |              | URL specifies the endpoint to check for the app health.                                                                                                                                                                                        |
| `»»» icon`                      | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                      | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» subdomain`                 | boolean                                                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»» subdomain_name`            | string                                                                                                 | false    |              | Subdomain name is the application domain exposed on the `coder server`.                                                                                                                                                                        |
| `»»» url`                       | string                                                                                                 | false    |              | URL is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |
| `»» architecture`               | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» connection_timeout_seconds` | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» directory`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» disconnected_at`            | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» display_apps`               | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» environment_variables`      | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» expanded_directory`         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» first_connected_at`         | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» health`                     | [codersdk.WorkspaceAgentHealth](schemas.md#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                                                                                        |
| `»»» healthy`                   | boolean                                                                                                | false    |              | Healthy is true if the agent is healthy.                                                                                                                                                                                                       |
| `»»» reason`                    | string                                                                                                 | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.                                                                                                                                                  |
| `»» id`                         | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» instance_id`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» last_connected_at`          | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» latency`                    | object                                                                                                 | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                            |
| `»»» [any property]`            | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» latency_ms`               | number                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» preferred`                | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» lifecycle_state`            | [codersdk.WorkspaceAgentLifecycle](schemas.md#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                                                                                                |
| `»» log_sources`                | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» created_at`                | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» icon`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» workspace_agent_id`        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» logs_length`                | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» logs_overflowed`            | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» operating_system`           | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» ready_at`                   | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» resource_id`                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» scripts`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» cron`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» log_path`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» log_source_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» run_on_start`              | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» run_on_stop`               | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» script`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» start_blocks_login`        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» timeout`                   | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» started_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script_behavior`    | [codersdk.WorkspaceAgentStartupScriptBehavior](schemas.md#codersdkworkspaceagentstartupscriptbehavior) | false    |              | Startup script behavior is a legacy field that is deprecated in favor of the `coder_script` resource. It's only referenced by old clients. Deprecated: Remove in the future!                                                                   |
| `»» status`                     | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»» subsystems`                 | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» troubleshooting_url`        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» updated_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» version`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                  | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                  | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» hide`                        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» icon`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» id`                          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» job_id`                      | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» metadata`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» key`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» sensitive`                  | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» value`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» name`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» type`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_transition`        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property                  | Value              |
| ------------------------- | ------------------ |
| `health`                  | `disabled`         |
| `health`                  | `initializing`     |
| `health`                  | `healthy`          |
| `health`                  | `unhealthy`        |
| `sharing_level`           | `owner`            |
| `sharing_level`           | `authenticated`    |
| `sharing_level`           | `public`           |
| `lifecycle_state`         | `created`          |
| `lifecycle_state`         | `starting`         |
| `lifecycle_state`         | `start_timeout`    |
| `lifecycle_state`         | `start_error`      |
| `lifecycle_state`         | `ready`            |
| `lifecycle_state`         | `shutting_down`    |
| `lifecycle_state`         | `shutdown_timeout` |
| `lifecycle_state`         | `shutdown_error`   |
| `lifecycle_state`         | `off`              |
| `startup_script_behavior` | `blocking`         |
| `startup_script_behavior` | `non-blocking`     |
| `status`                  | `connecting`       |
| `status`                  | `connected`        |
| `status`                  | `disconnected`     |
| `status`                  | `timeout`          |
| `workspace_transition`    | `start`            |
| `workspace_transition`    | `stop`             |
| `workspace_transition`    | `delete`           |

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
    "default_value": "string",
    "description": "string",
    "description_plaintext": "string",
    "display_name": "string",
    "ephemeral": true,
    "icon": "string",
    "mutable": true,
    "name": "string",
    "options": [
      {
        "description": "string",
        "icon": "string",
        "name": "string",
        "value": "string"
      }
    ],
    "required": true,
    "type": "string",
    "validation_error": "string",
    "validation_max": 0,
    "validation_min": 0,
    "validation_monotonic": "increasing",
    "validation_regex": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                    |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersionParameter](schemas.md#codersdktemplateversionparameter) |

<h3 id="get-rich-parameters-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name                      | Type                                                                             | Required | Restrictions | Description |
| ------------------------- | -------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]`            | array                                                                            | false    |              |             |
| `» default_value`         | string                                                                           | false    |              |             |
| `» description`           | string                                                                           | false    |              |             |
| `» description_plaintext` | string                                                                           | false    |              |             |
| `» display_name`          | string                                                                           | false    |              |             |
| `» ephemeral`             | boolean                                                                          | false    |              |             |
| `» icon`                  | string                                                                           | false    |              |             |
| `» mutable`               | boolean                                                                          | false    |              |             |
| `» name`                  | string                                                                           | false    |              |             |
| `» options`               | array                                                                            | false    |              |             |
| `»» description`          | string                                                                           | false    |              |             |
| `»» icon`                 | string                                                                           | false    |              |             |
| `»» name`                 | string                                                                           | false    |              |             |
| `»» value`                | string                                                                           | false    |              |             |
| `» required`              | boolean                                                                          | false    |              |             |
| `» type`                  | string                                                                           | false    |              |             |
| `» validation_error`      | string                                                                           | false    |              |             |
| `» validation_max`        | integer                                                                          | false    |              |             |
| `» validation_min`        | integer                                                                          | false    |              |             |
| `» validation_monotonic`  | [codersdk.ValidationMonotonicOrder](schemas.md#codersdkvalidationmonotonicorder) | false    |              |             |
| `» validation_regex`      | string                                                                           | false    |              |             |

#### Enumerated Values

| Property               | Value          |
| ---------------------- | -------------- |
| `type`                 | `string`       |
| `type`                 | `number`       |
| `type`                 | `bool`         |
| `type`                 | `list(string)` |
| `validation_monotonic` | `increasing`   |
| `validation_monotonic` | `decreasing`   |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Removed: Get schema by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/schema \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/schema`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Unarchive template version

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/templateversions/{templateversion}/unarchive \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /templateversions/{templateversion}/unarchive`

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

## Get template variables by template version

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templateversions/{templateversion}/variables \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templateversions/{templateversion}/variables`

### Parameters

| Name              | In   | Type         | Required | Description         |
| ----------------- | ---- | ------------ | -------- | ------------------- |
| `templateversion` | path | string(uuid) | true     | Template version ID |

### Example responses

> 200 Response

```json
[
  {
    "default_value": "string",
    "description": "string",
    "name": "string",
    "required": true,
    "sensitive": true,
    "type": "string",
    "value": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                  |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateVersionVariable](schemas.md#codersdktemplateversionvariable) |

<h3 id="get-template-variables-by-template-version-responseschema">Response Schema</h3>

Status Code **200**

| Name              | Type    | Required | Restrictions | Description |
| ----------------- | ------- | -------- | ------------ | ----------- |
| `[array item]`    | array   | false    |              |             |
| `» default_value` | string  | false    |              |             |
| `» description`   | string  | false    |              |             |
| `» name`          | string  | false    |              |             |
| `» required`      | boolean | false    |              |             |
| `» sensitive`     | boolean | false    |              |             |
| `» type`          | string  | false    |              |             |
| `» value`         | string  | false    |              |             |

#### Enumerated Values

| Property | Value    |
| -------- | -------- |
| `type`   | `string` |
| `type`   | `number` |
| `type`   | `bool`   |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
