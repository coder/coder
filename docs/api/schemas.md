# Schemas

> This page is incomplete, stay tuned.

## codersdk.AuthorizationCheck

```json
{
  "action": "create",
  "object": {
    "organization_id": "string",
    "owner_id": "string",
    "resource_id": "string",
    "resource_type": "string"
  }
}
```

AuthorizationCheck is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.

### Properties

| Name     | Type                                                         | Required | Restrictions | Description                                                                                                                                                |
| -------- | ------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `action` | string                                                       | false    |              |                                                                                                                                                            |
| `object` | [codersdk.AuthorizationObject](#codersdkauthorizationobject) | false    |              | Object can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, all workspaces across the entire product. |

#### Enumerated Values

| Property | Value  |
| -------- | ------ |
| action   | create |
| action   | read   |
| action   | update |
| action   | delete |

## codersdk.AuthorizationObject

```json
{
  "organization_id": "string",
  "owner_id": "string",
  "resource_id": "string",
  "resource_type": "string"
}
```

AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, all workspaces across the entire product.

### Properties

| Name              | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                            |
| ----------------- | ------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `organization_id` | string | false    |              | Organization id (optional) adds the set constraint to all resources owned by a given organization.                                                                                                                                                                                                                                                                     |
| `owner_id`        | string | false    |              | Owner id (optional) adds the set constraint to all resources owned by a given user.                                                                                                                                                                                                                                                                                    |
| `resource_id`     | string | false    |              | Resource id (optional) reduces the set to a singular resource. This assigns a resource ID to the resource type, eg: a single workspace. The rbac library will not fetch the resource from the database, so if you are using this option, you should also set the `OwnerID` and `OrganizationID` if possible. Be as specific as possible using all the fields relevant. |
| `resource_type`   | string | false    |              | Resource type is the name of the resource. `./coderd/rbac/object.go` has the list of valid resource types.                                                                                                                                                                                                                                                             |

## codersdk.AuthorizationRequest

```json
{
  "checks": {
    "property1": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    }
  }
}
```

### Properties

| Name               | Type                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                                                      |
| ------------------ | ---------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `checks`           | object                                                     | false    |              | Checks is a map keyed with an arbitrary string to a permission check. The key can be any string that is helpful to the caller, and allows multiple permission checks to be run in a single request. The key ensures that each permission check has the same key in the response. |
| » `[any property]` | [codersdk.AuthorizationCheck](#codersdkauthorizationcheck) | false    |              | It is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.                                                                                                                                                 |

## codersdk.AuthorizationResponse

```json
{
  "property1": true,
  "property2": true
}
```

### Properties

| Name             | Type    | Required | Restrictions | Description |
| ---------------- | ------- | -------- | ------------ | ----------- |
| `[any property]` | boolean | false    |              |             |

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

CreateParameterRequest is a structure used to create a new parameter value for a scope.

### Properties

| Name                  | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                        |
| --------------------- | ------ | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `copy_from_parameter` | string | false    |              | Copy from parameter allows copying the value of another parameter. The other param must be related to the same template_id for this to succeed. No other fields are required if using this, as all fields will be copied from the other parameter. |
| `destination_scheme`  | string | true     |              |                                                                                                                                                                                                                                                    |
| `name`                | string | true     |              |                                                                                                                                                                                                                                                    |
| `source_scheme`       | string | true     |              |                                                                                                                                                                                                                                                    |
| `source_value`        | string | true     |              |                                                                                                                                                                                                                                                    |

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

| Name                                                                                                                                                                                      | Type                                                                        | Required | Restrictions | Description                                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------- |
| `allow_user_cancel_workspace_jobs`                                                                                                                                                        | boolean                                                                     | false    |              | Allow users to cancel in-progress workspace jobs. \*bool as the default value is "true".                   |
| `default_ttl_ms`                                                                                                                                                                          | integer                                                                     | false    |              | Default ttl ms allows optionally specifying the default TTL for all workspaces created from this template. |
| `description`                                                                                                                                                                             | string                                                                      | false    |              | Description is a description of what the template contains. It must be less than 128 bytes.                |
| `display_name`                                                                                                                                                                            | string                                                                      | false    |              | Display name is the displayed name of the template.                                                        |
| `icon`                                                                                                                                                                                    | string                                                                      | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.           |
| `name`                                                                                                                                                                                    | string                                                                      | true     |              | Name is the name of the template.                                                                          |
| `parameter_values`                                                                                                                                                                        | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest) | false    |              | Parameter values is a structure used to create a new parameter value for a scope.]                         |
| `template_version_id`                                                                                                                                                                     | string                                                                      | true     |              | Template version id is an in-progress or completed job to use as an initial version of the template.       |
| This is required on creation to enable a user-flow of validating a template works. There is no reason the data-model cannot support empty templates, but it doesn't make sense for users. |

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
| `latency_ms` | number  | false    |              |             |
| `preferred`  | boolean | false    |              |             |

## codersdk.GetAppHostResponse

```json
{
  "host": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description                                                   |
| ------ | ------ | -------- | ------------ | ------------------------------------------------------------- |
| `host` | string | false    |              | Host is the externally accessible URL for the Coder instance. |

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
| `interval`  | integer | false    |              | Interval specifies the seconds between each health check.                                        |
| `threshold` | integer | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| `url`       | string  | false    |              | Url specifies the url to check for the app health.                                               |

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
| `canceled_at`      | string | false    |              |             |
| `completed_at`     | string | false    |              |             |
| `created_at`       | string | false    |              |             |
| `error`            | string | false    |              |             |
| `file_id`          | string | false    |              |             |
| `id`               | string | false    |              |             |
| `started_at`       | string | false    |              |             |
| `status`           | string | false    |              |             |
| `tags`             | object | false    |              |             |
| » `[any property]` | string | false    |              |             |
| `worker_id`        | string | false    |              |             |

## codersdk.PutExtendWorkspaceRequest

```json
{
  "deadline": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `deadline` | string | true     |              |             |

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

| Name          | Type                                                          | Required | Restrictions | Description                                                                                                                                                                                                                        |
| ------------- | ------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `detail`      | string                                                        | false    |              | Detail is a debug message that provides further insight into why the action failed. This information can be technical and a regular golang err.Error() text. - "database: too many open connections" - "stat: too many open files" |
| `message`     | string                                                        | false    |              | Message is an actionable message that depicts actions the request took. These messages should be fully formed sentences with proper punctuation. Examples: - "A user has been created." - "Failed to create a user."               |
| `validations` | array of [codersdk.ValidationError](#codersdkvalidationerror) | false    |              | Validations are form field-specific friendly error messages. They will be shown on a form field in the UI. These can also be used to add additional context if there is a set of errors in the primary 'Message'.                  |

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
| `active_user_count`                | integer                                                            | false    |              | Active user count is set to -1 when loading. |
| `active_version_id`                | string                                                             | false    |              |                                              |
| `allow_user_cancel_workspace_jobs` | boolean                                                            | false    |              |                                              |
| `build_time_stats`                 | [codersdk.TemplateBuildTimeStats](#codersdktemplatebuildtimestats) | false    |              |                                              |
| `created_at`                       | string                                                             | false    |              |                                              |
| `created_by_id`                    | string                                                             | false    |              |                                              |
| `created_by_name`                  | string                                                             | false    |              |                                              |
| `default_ttl_ms`                   | integer                                                            | false    |              |                                              |
| `description`                      | string                                                             | false    |              |                                              |
| `display_name`                     | string                                                             | false    |              |                                              |
| `icon`                             | string                                                             | false    |              |                                              |
| `id`                               | string                                                             | false    |              |                                              |
| `name`                             | string                                                             | false    |              |                                              |
| `organization_id`                  | string                                                             | false    |              |                                              |
| `provisioner`                      | string                                                             | false    |              |                                              |
| `updated_at`                       | string                                                             | false    |              |                                              |
| `workspace_owner_count`            | integer                                                            | false    |              |                                              |

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
| `[any property]` | [codersdk.TransitionStats](#codersdktransitionstats) | false    |              |             |

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
| `p50` | integer | false    |              |             |
| `p95` | integer | false    |              |             |

## codersdk.UpdateWorkspaceAutostartRequest

```json
{
  "schedule": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `schedule` | string | false    |              |             |

## codersdk.UpdateWorkspaceRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `name` | string | false    |              |             |

## codersdk.UpdateWorkspaceTTLRequest

```json
{
  "ttl_ms": 0
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
| -------- | ------- | -------- | ------------ | ----------- |
| `ttl_ms` | integer | false    |              |             |

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
| `detail` | string | true     |              |             |
| `field`  | string | true     |              |             |

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
| `autostart_schedule`                        | string                                             | false    |              |             |
| `created_at`                                | string                                             | false    |              |             |
| `id`                                        | string                                             | false    |              |             |
| `last_used_at`                              | string                                             | false    |              |             |
| `latest_build`                              | [codersdk.WorkspaceBuild](#codersdkworkspacebuild) | false    |              |             |
| `name`                                      | string                                             | false    |              |             |
| `outdated`                                  | boolean                                            | false    |              |             |
| `owner_id`                                  | string                                             | false    |              |             |
| `owner_name`                                | string                                             | false    |              |             |
| `template_allow_user_cancel_workspace_jobs` | boolean                                            | false    |              |             |
| `template_display_name`                     | string                                             | false    |              |             |
| `template_icon`                             | string                                             | false    |              |             |
| `template_id`                               | string                                             | false    |              |             |
| `template_name`                             | string                                             | false    |              |             |
| `ttl_ms`                                    | integer                                            | false    |              |             |
| `updated_at`                                | string                                             | false    |              |             |

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
| `apps`                       | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp) | false    |              |                                                                     |
| `architecture`               | string                                                  | false    |              |                                                                     |
| `connection_timeout_seconds` | integer                                                 | false    |              |                                                                     |
| `created_at`                 | string                                                  | false    |              |                                                                     |
| `directory`                  | string                                                  | false    |              |                                                                     |
| `disconnected_at`            | string                                                  | false    |              |                                                                     |
| `environment_variables`      | object                                                  | false    |              |                                                                     |
| » `[any property]`           | string                                                  | false    |              |                                                                     |
| `first_connected_at`         | string                                                  | false    |              |                                                                     |
| `id`                         | string                                                  | false    |              |                                                                     |
| `instance_id`                | string                                                  | false    |              |                                                                     |
| `last_connected_at`          | string                                                  | false    |              |                                                                     |
| `latency`                    | object                                                  | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle"). |
| » `[any property]`           | [codersdk.DERPRegion](#codersdkderpregion)              | false    |              |                                                                     |
| `name`                       | string                                                  | false    |              |                                                                     |
| `operating_system`           | string                                                  | false    |              |                                                                     |
| `resource_id`                | string                                                  | false    |              |                                                                     |
| `startup_script`             | string                                                  | false    |              |                                                                     |
| `status`                     | string                                                  | false    |              |                                                                     |
| `troubleshooting_url`        | string                                                  | false    |              |                                                                     |
| `updated_at`                 | string                                                  | false    |              |                                                                     |
| `version`                    | string                                                  | false    |              |                                                                     |

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

| Name            | Type                                         | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| --------------- | -------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `command`       | string                                       | false    |              |                                                                                                                                                                                                                                                |
| `display_name`  | string                                       | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `external`      | boolean                                      | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `health`        | string                                       | false    |              |                                                                                                                                                                                                                                                |
| `healthcheck`   | [codersdk.Healthcheck](#codersdkhealthcheck) | false    |              |                                                                                                                                                                                                                                                |
| `icon`          | string                                       | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `id`            | string                                       | false    |              |                                                                                                                                                                                                                                                |
| `sharing_level` | string                                       | false    |              |                                                                                                                                                                                                                                                |
| `slug`          | string                                       | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `subdomain`     | boolean                                      | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `url`           | string                                       | false    |              | Url is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |

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
| `build_number`          | integer                                                           | false    |              |             |
| `created_at`            | string                                                            | false    |              |             |
| `daily_cost`            | integer                                                           | false    |              |             |
| `deadline`              | string(time) or `null`                                            | false    |              |             |
| `id`                    | string                                                            | false    |              |             |
| `initiator_id`          | string                                                            | false    |              |             |
| `initiator_name`        | string                                                            | false    |              |             |
| `job`                   | [codersdk.ProvisionerJob](#codersdkprovisionerjob)                | false    |              |             |
| `reason`                | string                                                            | false    |              |             |
| `resources`             | array of [codersdk.WorkspaceResource](#codersdkworkspaceresource) | false    |              |             |
| `status`                | string                                                            | false    |              |             |
| `template_version_id`   | string                                                            | false    |              |             |
| `template_version_name` | string                                                            | false    |              |             |
| `transition`            | string                                                            | false    |              |             |
| `updated_at`            | string                                                            | false    |              |             |
| `workspace_id`          | string                                                            | false    |              |             |
| `workspace_name`        | string                                                            | false    |              |             |
| `workspace_owner_id`    | string                                                            | false    |              |             |
| `workspace_owner_name`  | string                                                            | false    |              |             |

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
| `agents`               | array of [codersdk.WorkspaceAgent](#codersdkworkspaceagent)                       | false    |              |             |
| `created_at`           | string                                                                            | false    |              |             |
| `daily_cost`           | integer                                                                           | false    |              |             |
| `hide`                 | boolean                                                                           | false    |              |             |
| `icon`                 | string                                                                            | false    |              |             |
| `id`                   | string                                                                            | false    |              |             |
| `job_id`               | string                                                                            | false    |              |             |
| `metadata`             | array of [codersdk.WorkspaceResourceMetadata](#codersdkworkspaceresourcemetadata) | false    |              |             |
| `name`                 | string                                                                            | false    |              |             |
| `type`                 | string                                                                            | false    |              |             |
| `workspace_transition` | string                                                                            | false    |              |             |

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
| `key`       | string  | false    |              |             |
| `sensitive` | boolean | false    |              |             |
| `value`     | string  | false    |              |             |

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
| `count`      | integer                                           | false    |              |             |
| `workspaces` | array of [codersdk.Workspace](#codersdkworkspace) | false    |              |             |
