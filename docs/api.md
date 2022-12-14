# Authentication

- API Key (CoderSessionToken)
  - Parameter Name: **Coder-Session-Token**, in: header.

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

| Name            | In   | Type                             | Required | Description     |
| --------------- | ---- | -------------------------------- | -------- | --------------- |
| organization-id | path | string                           | true     | Organization ID |
| body            | body | `codersdk.CreateTemplateRequest` | true     | Request body    |

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

| Status | Meaning                                                                    | Description           | Schema              |
| ------ | -------------------------------------------------------------------------- | --------------------- | ------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | `codersdk.Template` |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | `codersdk.Response` |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | `codersdk.Response` |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get template metadata

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

| Status | Meaning                                                                    | Description           | Schema              |
| ------ | -------------------------------------------------------------------------- | --------------------- | ------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | `codersdk.Template` |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | `codersdk.Response` |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | `codersdk.Response` |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

# Workspaces

## List workspaces

### Code samples

```shell
# You can also use wget
curl -X GET http://coder-server:8080/api/v2/workspaces \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /workspaces`

### Parameters

| Name      | In    | Type    | Required | Description        |
| --------- | ----- | ------- | -------- | ------------------ |
| owner     | query | string  | false    | Owner username     |
| template  | query | string  | false    | Template name      |
| name      | query | string  | false    | Workspace name     |
| status    | query | string  | false    | Workspace status   |
| deleted   | query | boolean | false    | Deleted workspaces |
| has_agent | query | boolean | false    | Has agent          |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "string",
      "id": "string",
      "last_used_at": "string",
      "latest_build": {
        "build_number": 0,
        "created_at": "string",
        "daily_cost": 0,
        "deadline": {
          "time": "string",
          "valid": true
        },
        "id": "string",
        "initiator_id": "string",
        "initiator_name": "string",
        "job": {
          "canceled_at": "string",
          "completed_at": "string",
          "created_at": "string",
          "error": "string",
          "file_id": "string",
          "id": "string",
          "started_at": "string",
          "status": "string",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "worker_id": "string"
        },
        "reason": "string",
        "resources": [
          {
            "agents": [
              {
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "health": "string",
                    "healthcheck": {},
                    "icon": "string",
                    "id": "string",
                    "sharing_level": "string",
                    "slug": "string",
                    "subdomain": true
                  }
                ],
                "architecture": "string",
                "connection_timeout_seconds": 0,
                "created_at": "string",
                "directory": "string",
                "disconnected_at": "string",
                "environment_variables": {
                  "property1": "string",
                  "property2": "string"
                },
                "first_connected_at": "string",
                "id": "string",
                "instance_id": "string",
                "last_connected_at": "string",
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
                "resource_id": "string",
                "startup_script": "string",
                "status": "string",
                "troubleshooting_url": "string",
                "updated_at": "string",
                "version": "string"
              }
            ],
            "created_at": "string",
            "daily_cost": 0,
            "hide": true,
            "icon": "string",
            "id": "string",
            "job_id": "string",
            "metadata": [
              {
                "key": "string",
                "sensitive": true,
                "value": "string"
              }
            ],
            "name": "string",
            "type": "string",
            "workspace_transition": "string"
          }
        ],
        "status": "string",
        "template_version_id": "string",
        "template_version_name": "string",
        "transition": "string",
        "updated_at": "string",
        "workspace_id": "string",
        "workspace_name": "string",
        "workspace_owner_id": "string",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "string",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "string",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                                    | Description           | Schema                        |
| ------ | -------------------------------------------------------------------------- | --------------------- | ----------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | `codersdk.WorkspacesResponse` |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request           | `codersdk.Response`           |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | `codersdk.Response`           |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get workspace metadata

### Code samples

```shell
# You can also use wget
curl -X GET http://coder-server:8080/api/v2/workspaces/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /workspaces/{id}`

### Parameters

| Name            | In    | Type   | Required | Description     |
| --------------- | ----- | ------ | -------- | --------------- |
| id              | path  | string | true     | Workspace ID    |
| include_deleted | query | string | false    | Include deleted |

### Example responses

> 200 Response

```json
{
  "autostart_schedule": "string",
  "created_at": "string",
  "id": "string",
  "last_used_at": "string",
  "latest_build": {
    "build_number": 0,
    "created_at": "string",
    "daily_cost": 0,
    "deadline": {
      "time": "string",
      "valid": true
    },
    "id": "string",
    "initiator_id": "string",
    "initiator_name": "string",
    "job": {
      "canceled_at": "string",
      "completed_at": "string",
      "created_at": "string",
      "error": "string",
      "file_id": "string",
      "id": "string",
      "started_at": "string",
      "status": "string",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "string"
    },
    "reason": "string",
    "resources": [
      {
        "agents": [
          {
            "apps": [
              {
                "command": "string",
                "display_name": "string",
                "health": "string",
                "healthcheck": {
                  "interval": 0,
                  "threshold": 0,
                  "url": "string"
                },
                "icon": "string",
                "id": "string",
                "sharing_level": "string",
                "slug": "string",
                "subdomain": true
              }
            ],
            "architecture": "string",
            "connection_timeout_seconds": 0,
            "created_at": "string",
            "directory": "string",
            "disconnected_at": "string",
            "environment_variables": {
              "property1": "string",
              "property2": "string"
            },
            "first_connected_at": "string",
            "id": "string",
            "instance_id": "string",
            "last_connected_at": "string",
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
            "resource_id": "string",
            "startup_script": "string",
            "status": "string",
            "troubleshooting_url": "string",
            "updated_at": "string",
            "version": "string"
          }
        ],
        "created_at": "string",
        "daily_cost": 0,
        "hide": true,
        "icon": "string",
        "id": "string",
        "job_id": "string",
        "metadata": [
          {
            "key": "string",
            "sensitive": true,
            "value": "string"
          }
        ],
        "name": "string",
        "type": "string",
        "workspace_transition": "string"
      }
    ],
    "status": "string",
    "template_version_id": "string",
    "template_version_name": "string",
    "transition": "string",
    "updated_at": "string",
    "workspace_id": "string",
    "workspace_name": "string",
    "workspace_owner_id": "string",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "string",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "string",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "string"
}
```

### Responses

| Status | Meaning                                                                    | Description           | Schema               |
| ------ | -------------------------------------------------------------------------- | --------------------- | -------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | `codersdk.Workspace` |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request           | `codersdk.Response`  |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | `codersdk.Response`  |
| 410    | [Gone](https://tools.ietf.org/html/rfc7231#section-6.5.9)                  | Gone                  | `codersdk.Response`  |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | `codersdk.Response`  |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

# Schemas

## codersdk.CreateParameterRequest

```json
{
  "copy_from_parameter": "string",
  "destination_scheme": "environment_variable",
  "name": "string",
  "source_scheme": "data",
  "source_value": "string"
}
```

### Properties

| Name                | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                        |
| ------------------- | ------ | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| copy_from_parameter | string | false    | none         | CloneID allows copying the value of another parameter.<br>The other param must be related to the same template_id for this to<br>succeed.<br>No other fields are required if using this, as all fields will be copied<br>from the other parameter. |
| destination_scheme  | string | true     | none         | none                                                                                                                                                                                                                                               |
| name                | string | true     | none         | none                                                                                                                                                                                                                                               |
| source_scheme       | string | true     | none         | none                                                                                                                                                                                                                                               |
| source_value        | string | true     | none         | none                                                                                                                                                                                                                                               |

#### Enumerated Values

| Property           | Value                |
| ------------------ | -------------------- |
| destination_scheme | environment_variable |
| destination_scheme | provisioner_variable |
| source_scheme      | data                 |

## codersdk.CreateTemplateRequest

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

### Properties

| Name                             | Type    | Required | Restrictions | Description                                                                                                                                                                                                                                                                                          |
| -------------------------------- | ------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| allow_user_cancel_workspace_jobs | boolean | false    | none         | Allow users to cancel in-progress workspace jobs.<br>\*bool as the default value is "true".                                                                                                                                                                                                          |
| default_ttl_ms                   | integer | false    | none         | DefaultTTLMillis allows optionally specifying the default TTL<br>for all workspaces created from this template.                                                                                                                                                                                      |
| description                      | string  | false    | none         | Description is a description of what the template contains. It must be<br>less than 128 bytes.                                                                                                                                                                                                       |
| display_name                     | string  | false    | none         | DisplayName is the displayed name of the template.                                                                                                                                                                                                                                                   |
| icon                             | string  | false    | none         | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                                                                  |
| name                             | string  | true     | none         | Name is the name of the template.                                                                                                                                                                                                                                                                    |
| parameter_values                 | array   | false    | none         | none                                                                                                                                                                                                                                                                                                 |
| template_version_id              | string  | true     | none         | VersionID is an in-progress or completed job to use as an initial version<br>of the template.<br><br>This is required on creation to enable a user-flow of validating a<br>template works. There is no reason the data-model cannot support empty<br>templates, but it doesn't make sense for users. |

## codersdk.DERPRegion

```json
{
  "latency_ms": 0,
  "preferred": true
}
```

### Properties

| Name       | Type    | Required | Restrictions | Description |
| ---------- | ------- | -------- | ------------ | ----------- |
| latency_ms | number  | false    | none         | none        |
| preferred  | boolean | false    | none         | none        |

## codersdk.Healthcheck

```json
{
  "interval": 0,
  "threshold": 0,
  "url": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description                                                                                      |
| --------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------ |
| interval  | integer | false    | none         | Interval specifies the seconds between each health check.                                        |
| threshold | integer | false    | none         | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| url       | string  | false    | none         | URL specifies the url to check for the app health.                                               |

## codersdk.NullTime

```json
{
  "time": "string",
  "valid": true
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description                       |
| ----- | ------- | -------- | ------------ | --------------------------------- |
| time  | string  | false    | none         | none                              |
| valid | boolean | false    | none         | Valid is true if Time is not NULL |

## codersdk.ProvisionerJob

```json
{
  "canceled_at": "string",
  "completed_at": "string",
  "created_at": "string",
  "error": "string",
  "file_id": "string",
  "id": "string",
  "started_at": "string",
  "status": "string",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "worker_id": "string"
}
```

### Properties

| Name                       | Type   | Required | Restrictions | Description |
| -------------------------- | ------ | -------- | ------------ | ----------- |
| canceled_at                | string | false    | none         | none        |
| completed_at               | string | false    | none         | none        |
| created_at                 | string | false    | none         | none        |
| error                      | string | false    | none         | none        |
| file_id                    | string | false    | none         | none        |
| id                         | string | false    | none         | none        |
| started_at                 | string | false    | none         | none        |
| status                     | string | false    | none         | none        |
| tags                       | object | false    | none         | none        |
| » **additionalProperties** | string | false    | none         | none        |
| worker_id                  | string | false    | none         | none        |

## codersdk.Response

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

### Properties

| Name        | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ----------- | ------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| detail      | string | false    | none         | Detail is a debug message that provides further insight into why the<br>action failed. This information can be technical and a regular golang<br>err.Error() text.<br>- "database: too many open connections"<br>- "stat: too many open files" |
| message     | string | false    | none         | Message is an actionable message that depicts actions the request took.<br>These messages should be fully formed sentences with proper punctuation.<br>Examples:<br>- "A user has been created."<br>- "Failed to create a user."               |
| validations | array  | false    | none         | Validations are form field-specific friendly error messages. They will be<br>shown on a form field in the UI. These can also be used to add additional<br>context if there is a set of errors in the primary 'Message'.                        |

## codersdk.Template

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

### Properties

| Name                             | Type                              | Required | Restrictions | Description                                |
| -------------------------------- | --------------------------------- | -------- | ------------ | ------------------------------------------ |
| active_user_count                | integer                           | false    | none         | ActiveUserCount is set to -1 when loading. |
| active_version_id                | string                            | false    | none         | none                                       |
| allow_user_cancel_workspace_jobs | boolean                           | false    | none         | none                                       |
| build_time_stats                 | `codersdk.TemplateBuildTimeStats` | false    | none         | none                                       |
| created_at                       | string                            | false    | none         | none                                       |
| created_by_id                    | string                            | false    | none         | none                                       |
| created_by_name                  | string                            | false    | none         | none                                       |
| default_ttl_ms                   | integer                           | false    | none         | none                                       |
| description                      | string                            | false    | none         | none                                       |
| display_name                     | string                            | false    | none         | none                                       |
| icon                             | string                            | false    | none         | none                                       |
| id                               | string                            | false    | none         | none                                       |
| name                             | string                            | false    | none         | none                                       |
| organization_id                  | string                            | false    | none         | none                                       |
| provisioner                      | string                            | false    | none         | none                                       |
| updated_at                       | string                            | false    | none         | none                                       |
| workspace_owner_count            | integer                           | false    | none         | none                                       |

## codersdk.TemplateBuildTimeStats

```json
{
  "property1": {
    "p50": 0,
    "p95": 0
  },
  "property2": {
    "p50": 0,
    "p95": 0
  }
}
```

### Properties

| Name                     | Type                       | Required | Restrictions | Description |
| ------------------------ | -------------------------- | -------- | ------------ | ----------- |
| **additionalProperties** | `codersdk.TransitionStats` | false    | none         | none        |

## codersdk.TransitionStats

```json
{
  "p50": 0,
  "p95": 0
}
```

### Properties

| Name | Type    | Required | Restrictions | Description |
| ---- | ------- | -------- | ------------ | ----------- |
| p50  | integer | false    | none         | none        |
| p95  | integer | false    | none         | none        |

## codersdk.ValidationError

```json
{
  "detail": "string",
  "field": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| detail | string | true     | none         | none        |
| field  | string | true     | none         | none        |

## codersdk.Workspace

```json
{
  "autostart_schedule": "string",
  "created_at": "string",
  "id": "string",
  "last_used_at": "string",
  "latest_build": {
    "build_number": 0,
    "created_at": "string",
    "daily_cost": 0,
    "deadline": {
      "time": "string",
      "valid": true
    },
    "id": "string",
    "initiator_id": "string",
    "initiator_name": "string",
    "job": {
      "canceled_at": "string",
      "completed_at": "string",
      "created_at": "string",
      "error": "string",
      "file_id": "string",
      "id": "string",
      "started_at": "string",
      "status": "string",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "string"
    },
    "reason": "string",
    "resources": [
      {
        "agents": [
          {
            "apps": [
              {
                "command": "string",
                "display_name": "string",
                "health": "string",
                "healthcheck": {
                  "interval": 0,
                  "threshold": 0,
                  "url": "string"
                },
                "icon": "string",
                "id": "string",
                "sharing_level": "string",
                "slug": "string",
                "subdomain": true
              }
            ],
            "architecture": "string",
            "connection_timeout_seconds": 0,
            "created_at": "string",
            "directory": "string",
            "disconnected_at": "string",
            "environment_variables": {
              "property1": "string",
              "property2": "string"
            },
            "first_connected_at": "string",
            "id": "string",
            "instance_id": "string",
            "last_connected_at": "string",
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
            "resource_id": "string",
            "startup_script": "string",
            "status": "string",
            "troubleshooting_url": "string",
            "updated_at": "string",
            "version": "string"
          }
        ],
        "created_at": "string",
        "daily_cost": 0,
        "hide": true,
        "icon": "string",
        "id": "string",
        "job_id": "string",
        "metadata": [
          {
            "key": "string",
            "sensitive": true,
            "value": "string"
          }
        ],
        "name": "string",
        "type": "string",
        "workspace_transition": "string"
      }
    ],
    "status": "string",
    "template_version_id": "string",
    "template_version_name": "string",
    "transition": "string",
    "updated_at": "string",
    "workspace_id": "string",
    "workspace_name": "string",
    "workspace_owner_id": "string",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "string",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "string",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "string"
}
```

### Properties

| Name                                      | Type                      | Required | Restrictions | Description |
| ----------------------------------------- | ------------------------- | -------- | ------------ | ----------- |
| autostart_schedule                        | string                    | false    | none         | none        |
| created_at                                | string                    | false    | none         | none        |
| id                                        | string                    | false    | none         | none        |
| last_used_at                              | string                    | false    | none         | none        |
| latest_build                              | `codersdk.WorkspaceBuild` | false    | none         | none        |
| name                                      | string                    | false    | none         | none        |
| outdated                                  | boolean                   | false    | none         | none        |
| owner_id                                  | string                    | false    | none         | none        |
| owner_name                                | string                    | false    | none         | none        |
| template_allow_user_cancel_workspace_jobs | boolean                   | false    | none         | none        |
| template_display_name                     | string                    | false    | none         | none        |
| template_icon                             | string                    | false    | none         | none        |
| template_id                               | string                    | false    | none         | none        |
| template_name                             | string                    | false    | none         | none        |
| ttl_ms                                    | integer                   | false    | none         | none        |
| updated_at                                | string                    | false    | none         | none        |

## codersdk.WorkspaceAgent

```json
{
  "apps": [
    {
      "command": "string",
      "display_name": "string",
      "health": "string",
      "healthcheck": {
        "interval": 0,
        "threshold": 0,
        "url": "string"
      },
      "icon": "string",
      "id": "string",
      "sharing_level": "string",
      "slug": "string",
      "subdomain": true
    }
  ],
  "architecture": "string",
  "connection_timeout_seconds": 0,
  "created_at": "string",
  "directory": "string",
  "disconnected_at": "string",
  "environment_variables": {
    "property1": "string",
    "property2": "string"
  },
  "first_connected_at": "string",
  "id": "string",
  "instance_id": "string",
  "last_connected_at": "string",
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
  "resource_id": "string",
  "startup_script": "string",
  "status": "string",
  "troubleshooting_url": "string",
  "updated_at": "string",
  "version": "string"
}
```

### Properties

| Name                       | Type                  | Required | Restrictions | Description                                                             |
| -------------------------- | --------------------- | -------- | ------------ | ----------------------------------------------------------------------- |
| apps                       | array                 | false    | none         | none                                                                    |
| architecture               | string                | false    | none         | none                                                                    |
| connection_timeout_seconds | integer               | false    | none         | none                                                                    |
| created_at                 | string                | false    | none         | none                                                                    |
| directory                  | string                | false    | none         | none                                                                    |
| disconnected_at            | string                | false    | none         | none                                                                    |
| environment_variables      | object                | false    | none         | none                                                                    |
| » **additionalProperties** | string                | false    | none         | none                                                                    |
| first_connected_at         | string                | false    | none         | none                                                                    |
| id                         | string                | false    | none         | none                                                                    |
| instance_id                | string                | false    | none         | none                                                                    |
| last_connected_at          | string                | false    | none         | none                                                                    |
| latency                    | object                | false    | none         | DERPLatency is mapped by region name (e.g. "New York City", "Seattle"). |
| » **additionalProperties** | `codersdk.DERPRegion` | false    | none         | none                                                                    |
| name                       | string                | false    | none         | none                                                                    |
| operating_system           | string                | false    | none         | none                                                                    |
| resource_id                | string                | false    | none         | none                                                                    |
| startup_script             | string                | false    | none         | none                                                                    |
| status                     | string                | false    | none         | none                                                                    |
| troubleshooting_url        | string                | false    | none         | none                                                                    |
| updated_at                 | string                | false    | none         | none                                                                    |
| version                    | string                | false    | none         | none                                                                    |

## codersdk.WorkspaceApp

```json
{
  "command": "string",
  "display_name": "string",
  "health": "string",
  "healthcheck": {
    "interval": 0,
    "threshold": 0,
    "url": "string"
  },
  "icon": "string",
  "id": "string",
  "sharing_level": "string",
  "slug": "string",
  "subdomain": true
}
```

### Properties

| Name          | Type                   | Required | Restrictions | Description                                                                                                                                                                                                                                             |
| ------------- | ---------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| command       | string                 | false    | none         | none                                                                                                                                                                                                                                                    |
| display_name  | string                 | false    | none         | DisplayName is a friendly name for the app.                                                                                                                                                                                                             |
| health        | string                 | false    | none         | none                                                                                                                                                                                                                                                    |
| healthcheck   | `codersdk.Healthcheck` | false    | none         | none                                                                                                                                                                                                                                                    |
| icon          | string                 | false    | none         | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                     |
| id            | string                 | false    | none         | none                                                                                                                                                                                                                                                    |
| sharing_level | string                 | false    | none         | none                                                                                                                                                                                                                                                    |
| slug          | string                 | false    | none         | Slug is a unique identifier within the agent.                                                                                                                                                                                                           |
| subdomain     | boolean                | false    | none         | Subdomain denotes whether the app should be accessed via a path on the<br>`coder server` or via a hostname-based dev URL. If this is set to true<br>and there is no app wildcard configured on the server, the app will not<br>be accessible in the UI. |

## codersdk.WorkspaceBuild

```json
{
  "build_number": 0,
  "created_at": "string",
  "daily_cost": 0,
  "deadline": {
    "time": "string",
    "valid": true
  },
  "id": "string",
  "initiator_id": "string",
  "initiator_name": "string",
  "job": {
    "canceled_at": "string",
    "completed_at": "string",
    "created_at": "string",
    "error": "string",
    "file_id": "string",
    "id": "string",
    "started_at": "string",
    "status": "string",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "string"
  },
  "reason": "string",
  "resources": [
    {
      "agents": [
        {
          "apps": [
            {
              "command": "string",
              "display_name": "string",
              "health": "string",
              "healthcheck": {
                "interval": 0,
                "threshold": 0,
                "url": "string"
              },
              "icon": "string",
              "id": "string",
              "sharing_level": "string",
              "slug": "string",
              "subdomain": true
            }
          ],
          "architecture": "string",
          "connection_timeout_seconds": 0,
          "created_at": "string",
          "directory": "string",
          "disconnected_at": "string",
          "environment_variables": {
            "property1": "string",
            "property2": "string"
          },
          "first_connected_at": "string",
          "id": "string",
          "instance_id": "string",
          "last_connected_at": "string",
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
          "resource_id": "string",
          "startup_script": "string",
          "status": "string",
          "troubleshooting_url": "string",
          "updated_at": "string",
          "version": "string"
        }
      ],
      "created_at": "string",
      "daily_cost": 0,
      "hide": true,
      "icon": "string",
      "id": "string",
      "job_id": "string",
      "metadata": [
        {
          "key": "string",
          "sensitive": true,
          "value": "string"
        }
      ],
      "name": "string",
      "type": "string",
      "workspace_transition": "string"
    }
  ],
  "status": "string",
  "template_version_id": "string",
  "template_version_name": "string",
  "transition": "string",
  "updated_at": "string",
  "workspace_id": "string",
  "workspace_name": "string",
  "workspace_owner_id": "string",
  "workspace_owner_name": "string"
}
```

### Properties

| Name                  | Type                      | Required | Restrictions | Description |
| --------------------- | ------------------------- | -------- | ------------ | ----------- |
| build_number          | integer                   | false    | none         | none        |
| created_at            | string                    | false    | none         | none        |
| daily_cost            | integer                   | false    | none         | none        |
| deadline              | `codersdk.NullTime`       | false    | none         | none        |
| id                    | string                    | false    | none         | none        |
| initiator_id          | string                    | false    | none         | none        |
| initiator_name        | string                    | false    | none         | none        |
| job                   | `codersdk.ProvisionerJob` | false    | none         | none        |
| reason                | string                    | false    | none         | none        |
| resources             | array                     | false    | none         | none        |
| status                | string                    | false    | none         | none        |
| template_version_id   | string                    | false    | none         | none        |
| template_version_name | string                    | false    | none         | none        |
| transition            | string                    | false    | none         | none        |
| updated_at            | string                    | false    | none         | none        |
| workspace_id          | string                    | false    | none         | none        |
| workspace_name        | string                    | false    | none         | none        |
| workspace_owner_id    | string                    | false    | none         | none        |
| workspace_owner_name  | string                    | false    | none         | none        |

## codersdk.WorkspaceResource

```json
{
  "agents": [
    {
      "apps": [
        {
          "command": "string",
          "display_name": "string",
          "health": "string",
          "healthcheck": {
            "interval": 0,
            "threshold": 0,
            "url": "string"
          },
          "icon": "string",
          "id": "string",
          "sharing_level": "string",
          "slug": "string",
          "subdomain": true
        }
      ],
      "architecture": "string",
      "connection_timeout_seconds": 0,
      "created_at": "string",
      "directory": "string",
      "disconnected_at": "string",
      "environment_variables": {
        "property1": "string",
        "property2": "string"
      },
      "first_connected_at": "string",
      "id": "string",
      "instance_id": "string",
      "last_connected_at": "string",
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
      "resource_id": "string",
      "startup_script": "string",
      "status": "string",
      "troubleshooting_url": "string",
      "updated_at": "string",
      "version": "string"
    }
  ],
  "created_at": "string",
  "daily_cost": 0,
  "hide": true,
  "icon": "string",
  "id": "string",
  "job_id": "string",
  "metadata": [
    {
      "key": "string",
      "sensitive": true,
      "value": "string"
    }
  ],
  "name": "string",
  "type": "string",
  "workspace_transition": "string"
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
| -------------------- | ------- | -------- | ------------ | ----------- |
| agents               | array   | false    | none         | none        |
| created_at           | string  | false    | none         | none        |
| daily_cost           | integer | false    | none         | none        |
| hide                 | boolean | false    | none         | none        |
| icon                 | string  | false    | none         | none        |
| id                   | string  | false    | none         | none        |
| job_id               | string  | false    | none         | none        |
| metadata             | array   | false    | none         | none        |
| name                 | string  | false    | none         | none        |
| type                 | string  | false    | none         | none        |
| workspace_transition | string  | false    | none         | none        |

## codersdk.WorkspaceResourceMetadata

```json
{
  "key": "string",
  "sensitive": true,
  "value": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
| --------- | ------- | -------- | ------------ | ----------- |
| key       | string  | false    | none         | none        |
| sensitive | boolean | false    | none         | none        |
| value     | string  | false    | none         | none        |

## codersdk.WorkspacesResponse

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "string",
      "id": "string",
      "last_used_at": "string",
      "latest_build": {
        "build_number": 0,
        "created_at": "string",
        "daily_cost": 0,
        "deadline": {
          "time": "string",
          "valid": true
        },
        "id": "string",
        "initiator_id": "string",
        "initiator_name": "string",
        "job": {
          "canceled_at": "string",
          "completed_at": "string",
          "created_at": "string",
          "error": "string",
          "file_id": "string",
          "id": "string",
          "started_at": "string",
          "status": "string",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "worker_id": "string"
        },
        "reason": "string",
        "resources": [
          {
            "agents": [
              {
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "health": "string",
                    "healthcheck": {},
                    "icon": "string",
                    "id": "string",
                    "sharing_level": "string",
                    "slug": "string",
                    "subdomain": true
                  }
                ],
                "architecture": "string",
                "connection_timeout_seconds": 0,
                "created_at": "string",
                "directory": "string",
                "disconnected_at": "string",
                "environment_variables": {
                  "property1": "string",
                  "property2": "string"
                },
                "first_connected_at": "string",
                "id": "string",
                "instance_id": "string",
                "last_connected_at": "string",
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
                "resource_id": "string",
                "startup_script": "string",
                "status": "string",
                "troubleshooting_url": "string",
                "updated_at": "string",
                "version": "string"
              }
            ],
            "created_at": "string",
            "daily_cost": 0,
            "hide": true,
            "icon": "string",
            "id": "string",
            "job_id": "string",
            "metadata": [
              {
                "key": "string",
                "sensitive": true,
                "value": "string"
              }
            ],
            "name": "string",
            "type": "string",
            "workspace_transition": "string"
          }
        ],
        "status": "string",
        "template_version_id": "string",
        "template_version_name": "string",
        "transition": "string",
        "updated_at": "string",
        "workspace_id": "string",
        "workspace_name": "string",
        "workspace_owner_id": "string",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "string",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "string",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "string"
    }
  ]
}
```

### Properties

| Name       | Type    | Required | Restrictions | Description |
| ---------- | ------- | -------- | ------------ | ----------- |
| count      | integer | false    | none         | none        |
| workspaces | array   | false    | none         | none        |
