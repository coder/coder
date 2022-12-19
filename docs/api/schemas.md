# Schemas

> This page is incomplete, stay tuned.

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

| Name                  | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                                    |
| --------------------- | ------ | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `copy_from_parameter` | string | false    | none         | Copy from parameter allows copying the value of another parameter.<br>The other param must be related to the same template_id for this to<br>succeed.<br>No other fields are required if using this, as all fields will be copied<br>from the other parameter. |
| `destination_scheme`  | string | true     | none         | none                                                                                                                                                                                                                                                           |
| `name`                | string | true     | none         | none                                                                                                                                                                                                                                                           |
| `source_scheme`       | string | true     | none         | none                                                                                                                                                                                                                                                           |
| `source_value`        | string | true     | none         | none                                                                                                                                                                                                                                                           |

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

| Name                               | Type                                                                        | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                    |
| ---------------------------------- | --------------------------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `allow_user_cancel_workspace_jobs` | boolean                                                                     | false    | none         | Allow users to cancel in-progress workspace jobs.<br>\*bool as the default value is "true".                                                                                                                                                                                                                    |
| `default_ttl_ms`                   | integer                                                                     | false    | none         | Default ttl ms allows optionally specifying the default TTL<br>for all workspaces created from this template.                                                                                                                                                                                                  |
| `description`                      | string                                                                      | false    | none         | Description is a description of what the template contains. It must be<br>less than 128 bytes.                                                                                                                                                                                                                 |
| `display_name`                     | string                                                                      | false    | none         | Display name is the displayed name of the template.                                                                                                                                                                                                                                                            |
| `icon`                             | string                                                                      | false    | none         | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                                                                            |
| `name`                             | string                                                                      | true     | none         | Name is the name of the template.                                                                                                                                                                                                                                                                              |
| `parameter_values`                 | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest) | false    | none         | none                                                                                                                                                                                                                                                                                                           |
| `template_version_id`              | string                                                                      | true     | none         | Template version id is an in-progress or completed job to use as an initial version<br>of the template.<br><br>This is required on creation to enable a user-flow of validating a<br>template works. There is no reason the data-model cannot support empty<br>templates, but it doesn't make sense for users. |

## codersdk.DERPRegion

```json
{
  "latency_ms": 0,
  "preferred": true
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `latency_ms` | number  | false    | none         | none        |
| `preferred`  | boolean | false    | none         | none        |

## codersdk.Healthcheck

```json
{
  "interval": 0,
  "threshold": 0,
  "url": "string"
}
```

### Properties

| Name        | Type    | Required | Restrictions | Description                                                                                      |
| ----------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------ |
| `interval`  | integer | false    | none         | Interval specifies the seconds between each health check.                                        |
| `threshold` | integer | false    | none         | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| `url`       | string  | false    | none         | Url specifies the url to check for the app health.                                               |

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

| Name               | Type   | Required | Restrictions | Description |
| ------------------ | ------ | -------- | ------------ | ----------- |
| `canceled_at`      | string | false    | none         | none        |
| `completed_at`     | string | false    | none         | none        |
| `created_at`       | string | false    | none         | none        |
| `error`            | string | false    | none         | none        |
| `file_id`          | string | false    | none         | none        |
| `id`               | string | false    | none         | none        |
| `started_at`       | string | false    | none         | none        |
| `status`           | string | false    | none         | none        |
| `tags`             | object | false    | none         | none        |
| » `[any property]` | string | false    | none         | none        |
| `worker_id`        | string | false    | none         | none        |

## codersdk.PutExtendWorkspaceRequest

```json
{
  "deadline": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `deadline` | string | true     | none         | none        |

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

| Name          | Type                                                          | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------- | ------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `detail`      | string                                                        | false    | none         | Detail is a debug message that provides further insight into why the<br>action failed. This information can be technical and a regular golang<br>err.Error() text.<br>- "database: too many open connections"<br>- "stat: too many open files" |
| `message`     | string                                                        | false    | none         | Message is an actionable message that depicts actions the request took.<br>These messages should be fully formed sentences with proper punctuation.<br>Examples:<br>- "A user has been created."<br>- "Failed to create a user."               |
| `validations` | array of [codersdk.ValidationError](#codersdkvalidationerror) | false    | none         | Validations are form field-specific friendly error messages. They will be<br>shown on a form field in the UI. These can also be used to add additional<br>context if there is a set of errors in the primary 'Message'.                        |

## codersdk.Template

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

### Properties

| Name                               | Type                                                               | Required | Restrictions | Description                                  |
| ---------------------------------- | ------------------------------------------------------------------ | -------- | ------------ | -------------------------------------------- |
| `active_user_count`                | integer                                                            | false    | none         | Active user count is set to -1 when loading. |
| `active_version_id`                | string                                                             | false    | none         | none                                         |
| `allow_user_cancel_workspace_jobs` | boolean                                                            | false    | none         | none                                         |
| `build_time_stats`                 | [codersdk.TemplateBuildTimeStats](#codersdktemplatebuildtimestats) | false    | none         | none                                         |
| `created_at`                       | string                                                             | false    | none         | none                                         |
| `created_by_id`                    | string                                                             | false    | none         | none                                         |
| `created_by_name`                  | string                                                             | false    | none         | none                                         |
| `default_ttl_ms`                   | integer                                                            | false    | none         | none                                         |
| `description`                      | string                                                             | false    | none         | none                                         |
| `display_name`                     | string                                                             | false    | none         | none                                         |
| `icon`                             | string                                                             | false    | none         | none                                         |
| `id`                               | string                                                             | false    | none         | none                                         |
| `name`                             | string                                                             | false    | none         | none                                         |
| `organization_id`                  | string                                                             | false    | none         | none                                         |
| `provisioner`                      | string                                                             | false    | none         | none                                         |
| `updated_at`                       | string                                                             | false    | none         | none                                         |
| `workspace_owner_count`            | integer                                                            | false    | none         | none                                         |

## codersdk.TemplateBuildTimeStats

```json
{
  "property1": {
    "p50": 123,
    "p95": 146
  },
  "property2": {
    "p50": 123,
    "p95": 146
  }
}
```

### Properties

| Name             | Type                                                 | Required | Restrictions | Description |
| ---------------- | ---------------------------------------------------- | -------- | ------------ | ----------- |
| `[any property]` | [codersdk.TransitionStats](#codersdktransitionstats) | false    | none         | none        |

## codersdk.TransitionStats

```json
{
  "p50": 123,
  "p95": 146
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description |
| ----- | ------- | -------- | ------------ | ----------- |
| `p50` | integer | false    | none         | none        |
| `p95` | integer | false    | none         | none        |

## codersdk.UpdateWorkspaceAutostartRequest

```json
{
  "schedule": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `schedule` | string | false    | none         | none        |

## codersdk.UpdateWorkspaceRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `name` | string | false    | none         | none        |

## codersdk.UpdateWorkspaceTTLRequest

```json
{
  "ttl_ms": 0
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
| -------- | ------- | -------- | ------------ | ----------- |
| `ttl_ms` | integer | false    | none         | none        |

## codersdk.ValidationError

```json
{
  "detail": "string",
  "field": "string"
}
```

### Properties

| Name     | Type   | Required | Restrictions | Description |
| -------- | ------ | -------- | ------------ | ----------- |
| `detail` | string | true     | none         | none        |
| `field`  | string | true     | none         | none        |

## codersdk.Workspace

```json
{
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_build": {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": {
      "time": "string",
      "valid": true
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
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
                "external": true,
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
                "subdomain": true,
                "url": "string"
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
    ],
    "status": "pending",
    "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
    "template_version_name": "string",
    "transition": "start",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string",
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                                        | Type                                               | Required | Restrictions | Description |
| ------------------------------------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `autostart_schedule`                        | string                                             | false    | none         | none        |
| `created_at`                                | string                                             | false    | none         | none        |
| `id`                                        | string                                             | false    | none         | none        |
| `last_used_at`                              | string                                             | false    | none         | none        |
| `latest_build`                              | [codersdk.WorkspaceBuild](#codersdkworkspacebuild) | false    | none         | none        |
| `name`                                      | string                                             | false    | none         | none        |
| `outdated`                                  | boolean                                            | false    | none         | none        |
| `owner_id`                                  | string                                             | false    | none         | none        |
| `owner_name`                                | string                                             | false    | none         | none        |
| `template_allow_user_cancel_workspace_jobs` | boolean                                            | false    | none         | none        |
| `template_display_name`                     | string                                             | false    | none         | none        |
| `template_icon`                             | string                                             | false    | none         | none        |
| `template_id`                               | string                                             | false    | none         | none        |
| `template_name`                             | string                                             | false    | none         | none        |
| `ttl_ms`                                    | integer                                            | false    | none         | none        |
| `updated_at`                                | string                                             | false    | none         | none        |

## codersdk.WorkspaceAgent

```json
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
      "id": "string",
      "sharing_level": "string",
      "slug": "string",
      "subdomain": true,
      "url": "string"
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

| Name                         | Type                                                    | Required | Restrictions | Description                                                         |
| ---------------------------- | ------------------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------- |
| `apps`                       | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp) | false    | none         | none                                                                |
| `architecture`               | string                                                  | false    | none         | none                                                                |
| `connection_timeout_seconds` | integer                                                 | false    | none         | none                                                                |
| `created_at`                 | string                                                  | false    | none         | none                                                                |
| `directory`                  | string                                                  | false    | none         | none                                                                |
| `disconnected_at`            | string                                                  | false    | none         | none                                                                |
| `environment_variables`      | object                                                  | false    | none         | none                                                                |
| » `[any property]`           | string                                                  | false    | none         | none                                                                |
| `first_connected_at`         | string                                                  | false    | none         | none                                                                |
| `id`                         | string                                                  | false    | none         | none                                                                |
| `instance_id`                | string                                                  | false    | none         | none                                                                |
| `last_connected_at`          | string                                                  | false    | none         | none                                                                |
| `latency`                    | object                                                  | false    | none         | Latency is mapped by region name (e.g. "New York City", "Seattle"). |
| » `[any property]`           | [codersdk.DERPRegion](#codersdkderpregion)              | false    | none         | none                                                                |
| `name`                       | string                                                  | false    | none         | none                                                                |
| `operating_system`           | string                                                  | false    | none         | none                                                                |
| `resource_id`                | string                                                  | false    | none         | none                                                                |
| `startup_script`             | string                                                  | false    | none         | none                                                                |
| `status`                     | string                                                  | false    | none         | none                                                                |
| `troubleshooting_url`        | string                                                  | false    | none         | none                                                                |
| `updated_at`                 | string                                                  | false    | none         | none                                                                |
| `version`                    | string                                                  | false    | none         | none                                                                |

## codersdk.WorkspaceApp

```json
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
  "id": "string",
  "sharing_level": "string",
  "slug": "string",
  "subdomain": true,
  "url": "string"
}
```

### Properties

| Name            | Type                                         | Required | Restrictions | Description                                                                                                                                                                                                                                             |
| --------------- | -------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `command`       | string                                       | false    | none         | none                                                                                                                                                                                                                                                    |
| `display_name`  | string                                       | false    | none         | Display name is a friendly name for the app.                                                                                                                                                                                                            |
| `external`      | boolean                                      | false    | none         | External specifies whether the URL should be opened externally on<br>the client or not.                                                                                                                                                                 |
| `health`        | string                                       | false    | none         | none                                                                                                                                                                                                                                                    |
| `healthcheck`   | [codersdk.Healthcheck](#codersdkhealthcheck) | false    | none         | none                                                                                                                                                                                                                                                    |
| `icon`          | string                                       | false    | none         | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                     |
| `id`            | string                                       | false    | none         | none                                                                                                                                                                                                                                                    |
| `sharing_level` | string                                       | false    | none         | none                                                                                                                                                                                                                                                    |
| `slug`          | string                                       | false    | none         | Slug is a unique identifier within the agent.                                                                                                                                                                                                           |
| `subdomain`     | boolean                                      | false    | none         | Subdomain denotes whether the app should be accessed via a path on the<br>`coder server` or via a hostname-based dev URL. If this is set to true<br>and there is no app wildcard configured on the server, the app will not<br>be accessible in the UI. |
| `url`           | string                                       | false    | none         | Url is the address being proxied to inside the workspace.<br>If external is specified, this will be opened on the client.                                                                                                                               |

## codersdk.WorkspaceBuild

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": {
    "time": "string",
    "valid": true
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
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
              "external": true,
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
              "subdomain": true,
              "url": "string"
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
  ],
  "status": "pending",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "template_version_name": "string",
  "transition": "start",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
  "workspace_name": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Properties

| Name                    | Type                                                              | Required | Restrictions | Description |
| ----------------------- | ----------------------------------------------------------------- | -------- | ------------ | ----------- |
| `build_number`          | integer                                                           | false    | none         | none        |
| `created_at`            | string                                                            | false    | none         | none        |
| `daily_cost`            | integer                                                           | false    | none         | none        |
| `deadline`              | string(time) or `null`                                            | false    | none         | none        |
| `id`                    | string                                                            | false    | none         | none        |
| `initiator_id`          | string                                                            | false    | none         | none        |
| `initiator_name`        | string                                                            | false    | none         | none        |
| `job`                   | [codersdk.ProvisionerJob](#codersdkprovisionerjob)                | false    | none         | none        |
| `reason`                | string                                                            | false    | none         | none        |
| `resources`             | array of [codersdk.WorkspaceResource](#codersdkworkspaceresource) | false    | none         | none        |
| `status`                | string                                                            | false    | none         | none        |
| `template_version_id`   | string                                                            | false    | none         | none        |
| `template_version_name` | string                                                            | false    | none         | none        |
| `transition`            | string                                                            | false    | none         | none        |
| `updated_at`            | string                                                            | false    | none         | none        |
| `workspace_id`          | string                                                            | false    | none         | none        |
| `workspace_name`        | string                                                            | false    | none         | none        |
| `workspace_owner_id`    | string                                                            | false    | none         | none        |
| `workspace_owner_name`  | string                                                            | false    | none         | none        |

#### Enumerated Values

| Property   | Value     |
| ---------- | --------- |
| status     | pending   |
| status     | starting  |
| status     | running   |
| status     | stopping  |
| status     | stopped   |
| status     | failed    |
| status     | canceling |
| status     | canceled  |
| status     | deleting  |
| status     | deleted   |
| transition | start     |
| transition | stop      |
| transition | delete    |

## codersdk.WorkspaceResource

```json
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
          "id": "string",
          "sharing_level": "string",
          "slug": "string",
          "subdomain": true,
          "url": "string"
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
```

### Properties

| Name                   | Type                                                                              | Required | Restrictions | Description |
| ---------------------- | --------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `agents`               | array of [codersdk.WorkspaceAgent](#codersdkworkspaceagent)                       | false    | none         | none        |
| `created_at`           | string                                                                            | false    | none         | none        |
| `daily_cost`           | integer                                                                           | false    | none         | none        |
| `hide`                 | boolean                                                                           | false    | none         | none        |
| `icon`                 | string                                                                            | false    | none         | none        |
| `id`                   | string                                                                            | false    | none         | none        |
| `job_id`               | string                                                                            | false    | none         | none        |
| `metadata`             | array of [codersdk.WorkspaceResourceMetadata](#codersdkworkspaceresourcemetadata) | false    | none         | none        |
| `name`                 | string                                                                            | false    | none         | none        |
| `type`                 | string                                                                            | false    | none         | none        |
| `workspace_transition` | string                                                                            | false    | none         | none        |

#### Enumerated Values

| Property             | Value  |
| -------------------- | ------ |
| workspace_transition | start  |
| workspace_transition | stop   |
| workspace_transition | delete |

## codersdk.WorkspaceResourceMetadata

```json
{
  "key": "string",
  "sensitive": true,
  "value": "string"
}
```

### Properties

| Name        | Type    | Required | Restrictions | Description |
| ----------- | ------- | -------- | ------------ | ----------- |
| `key`       | string  | false    | none         | none        |
| `sensitive` | boolean | false    | none         | none        |
| `value`     | string  | false    | none         | none        |

## codersdk.WorkspacesResponse

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_used_at": "2019-08-24T14:15:22Z",
      "latest_build": {
        "build_number": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "daily_cost": 0,
        "deadline": {
          "time": "string",
          "valid": true
        },
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
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
                    "external": true,
                    "health": "string",
                    "healthcheck": {},
                    "icon": "string",
                    "id": "string",
                    "sharing_level": "string",
                    "slug": "string",
                    "subdomain": true,
                    "url": "string"
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
        ],
        "status": "pending",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "template_version_name": "string",
        "transition": "start",
        "updated_at": "2019-08-24T14:15:22Z",
        "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
        "workspace_name": "string",
        "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name         | Type                                              | Required | Restrictions | Description |
| ------------ | ------------------------------------------------- | -------- | ------------ | ----------- |
| `count`      | integer                                           | false    | none         | none        |
| `workspaces` | array of [codersdk.Workspace](#codersdkworkspace) | false    | none         | none        |
