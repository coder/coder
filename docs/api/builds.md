# Builds

## Get workspace build by user, workspace name, and build number

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/workspace/{workspacename}/builds/{buildnumber} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/workspace/{workspacename}/builds/{buildnumber}`

### Parameters

| Name            | In   | Type           | Required | Description          |
| --------------- | ---- | -------------- | -------- | -------------------- |
| `user`          | path | string         | true     | User ID, name, or me |
| `workspacename` | path | string         | true     | Workspace name       |
| `buildnumber`   | path | string(number) | true     | Build number         |

### Example responses

> 200 Response

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
  "initiator_name": "string",
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
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
          "login_before_ready": true,
          "logs_length": 0,
          "logs_overflowed": true,
          "name": "string",
          "operating_system": "string",
          "ready_at": "2019-08-24T14:15:22Z",
          "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
          "shutdown_script": "string",
          "shutdown_script_timeout_seconds": 0,
          "started_at": "2019-08-24T14:15:22Z",
          "startup_script": "string",
          "startup_script_behavior": "blocking",
          "startup_script_timeout_seconds": 0,
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

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace build

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}`

### Parameters

| Name             | In   | Type   | Required | Description        |
| ---------------- | ---- | ------ | -------- | ------------------ |
| `workspacebuild` | path | string | true     | Workspace build ID |

### Example responses

> 200 Response

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
  "initiator_name": "string",
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
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
          "login_before_ready": true,
          "logs_length": 0,
          "logs_overflowed": true,
          "name": "string",
          "operating_system": "string",
          "ready_at": "2019-08-24T14:15:22Z",
          "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
          "shutdown_script": "string",
          "shutdown_script_timeout_seconds": 0,
          "started_at": "2019-08-24T14:15:22Z",
          "startup_script": "string",
          "startup_script_behavior": "blocking",
          "startup_script_timeout_seconds": 0,
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

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Cancel workspace build

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/cancel \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /workspacebuilds/{workspacebuild}/cancel`

### Parameters

| Name             | In   | Type   | Required | Description        |
| ---------------- | ---- | ------ | -------- | ------------------ |
| `workspacebuild` | path | string | true     | Workspace build ID |

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

## Get workspace build logs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}/logs`

### Parameters

| Name             | In    | Type    | Required | Description           |
| ---------------- | ----- | ------- | -------- | --------------------- |
| `workspacebuild` | path  | string  | true     | Workspace build ID    |
| `before`         | query | integer | false    | Before Unix timestamp |
| `after`          | query | integer | false    | After Unix timestamp  |
| `follow`         | query | boolean | false    | Follow log stream     |

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

<h3 id="get-workspace-build-logs-responseschema">Response Schema</h3>

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

## Get build parameters for workspace build

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/parameters \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}/parameters`

### Parameters

| Name             | In   | Type   | Required | Description        |
| ---------------- | ---- | ------ | -------- | ------------------ |
| `workspacebuild` | path | string | true     | Workspace build ID |

### Example responses

> 200 Response

```json
[
  {
    "name": "string",
    "value": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                  |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceBuildParameter](schemas.md#codersdkworkspacebuildparameter) |

<h3 id="get-build-parameters-for-workspace-build-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type   | Required | Restrictions | Description |
| -------------- | ------ | -------- | ------------ | ----------- |
| `[array item]` | array  | false    |              |             |
| `» name`       | string | false    |              |             |
| `» value`      | string | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace resources for workspace build

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/resources \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}/resources`

### Parameters

| Name             | In   | Type   | Required | Description        |
| ---------------- | ---- | ------ | -------- | ------------------ |
| `workspacebuild` | path | string | true     | Workspace build ID |

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
        "login_before_ready": true,
        "logs_length": 0,
        "logs_overflowed": true,
        "name": "string",
        "operating_system": "string",
        "ready_at": "2019-08-24T14:15:22Z",
        "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
        "shutdown_script": "string",
        "shutdown_script_timeout_seconds": 0,
        "started_at": "2019-08-24T14:15:22Z",
        "startup_script": "string",
        "startup_script_behavior": "blocking",
        "startup_script_timeout_seconds": 0,
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

<h3 id="get-workspace-resources-for-workspace-build-responseschema">Response Schema</h3>

Status Code **200**

| Name                                 | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                       | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `» agents`                           | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» apps`                            | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» command`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_name`                   | string                                                                                                 | false    |              | »»display name is a friendly name for the app.                                                                                                                                                                                                 |
| `»»» external`                       | boolean                                                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»» health`                         | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»» healthcheck`                    | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                                                 | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»» interval`                      | integer                                                                                                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»» threshold`                     | integer                                                                                                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»» url`                           | string                                                                                                 | false    |              | »»»url specifies the endpoint to check for the app health.                                                                                                                                                                                     |
| `»»» icon`                           | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`                  | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                           | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» subdomain`                      | boolean                                                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»» url`                            | string                                                                                                 | false    |              | »»url is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                       |
| `»» architecture`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» connection_timeout_seconds`      | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» directory`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» disconnected_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» display_apps`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» environment_variables`           | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`                 | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» expanded_directory`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» first_connected_at`              | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» health`                          | [codersdk.WorkspaceAgentHealth](schemas.md#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                                                                                        |
| `»»» healthy`                        | boolean                                                                                                | false    |              | Healthy is true if the agent is healthy.                                                                                                                                                                                                       |
| `»»» reason`                         | string                                                                                                 | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.                                                                                                                                                  |
| `»» id`                              | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» instance_id`                     | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» last_connected_at`               | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» latency`                         | object                                                                                                 | false    |              | »latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                           |
| `»»» [any property]`                 | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» latency_ms`                    | number                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» preferred`                     | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» lifecycle_state`                 | [codersdk.WorkspaceAgentLifecycle](schemas.md#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                                                                                                |
| `»» login_before_ready`              | boolean                                                                                                | false    |              | Deprecated: Use StartupScriptBehavior instead.                                                                                                                                                                                                 |
| `»» logs_length`                     | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» logs_overflowed`                 | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» operating_system`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» ready_at`                        | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» resource_id`                     | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» shutdown_script`                 | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» shutdown_script_timeout_seconds` | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» started_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script_behavior`         | [codersdk.WorkspaceAgentStartupScriptBehavior](schemas.md#codersdkworkspaceagentstartupscriptbehavior) | false    |              |                                                                                                                                                                                                                                                |
| `»» startup_script_timeout_seconds`  | integer                                                                                                | false    |              | »startup script timeout seconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.                                    |
| `»» status`                          | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»» subsystems`                      | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» troubleshooting_url`             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» updated_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» version`                         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                       | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                       | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» hide`                             | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» icon`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» id`                               | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» job_id`                           | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» metadata`                         | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» key`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» sensitive`                       | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» value`                           | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» name`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» type`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_transition`             | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |

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

## Get provisioner state for workspace build

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/state \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}/state`

### Parameters

| Name             | In   | Type   | Required | Description        |
| ---------------- | ---- | ------ | -------- | ------------------ |
| `workspacebuild` | path | string | true     | Workspace build ID |

### Example responses

> 200 Response

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
  "initiator_name": "string",
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
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
          "login_before_ready": true,
          "logs_length": 0,
          "logs_overflowed": true,
          "name": "string",
          "operating_system": "string",
          "ready_at": "2019-08-24T14:15:22Z",
          "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
          "shutdown_script": "string",
          "shutdown_script_timeout_seconds": 0,
          "started_at": "2019-08-24T14:15:22Z",
          "startup_script": "string",
          "startup_script_behavior": "blocking",
          "startup_script_timeout_seconds": 0,
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

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace builds by workspace ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace}/builds \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}/builds`

### Parameters

| Name        | In    | Type              | Required | Description     |
| ----------- | ----- | ----------------- | -------- | --------------- |
| `workspace` | path  | string(uuid)      | true     | Workspace ID    |
| `after_id`  | query | string(uuid)      | false    | After ID        |
| `limit`     | query | integer           | false    | Page limit      |
| `offset`    | query | integer           | false    | Page offset     |
| `since`     | query | string(date-time) | false    | Since timestamp |

### Example responses

> 200 Response

```json
[
  {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
    "initiator_name": "string",
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
    "max_deadline": "2019-08-24T14:15:22Z",
    "reason": "initiator",
    "resources": [
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
            "login_before_ready": true,
            "logs_length": 0,
            "logs_overflowed": true,
            "name": "string",
            "operating_system": "string",
            "ready_at": "2019-08-24T14:15:22Z",
            "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
            "shutdown_script": "string",
            "shutdown_script_timeout_seconds": 0,
            "started_at": "2019-08-24T14:15:22Z",
            "startup_script": "string",
            "startup_script_behavior": "blocking",
            "startup_script_timeout_seconds": 0,
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
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

<h3 id="get-workspace-builds-by-workspace-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                                  | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`                        | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `» build_number`                      | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                        | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                        | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» deadline`                          | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» id`                                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» initiator_id`                      | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» initiator_name`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» job`                               | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» canceled_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» completed_at`                     | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                       | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» error`                            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» error_code`                       | [codersdk.JobErrorCode](schemas.md#codersdkjoberrorcode)                                               | false    |              |                                                                                                                                                                                                                                                |
| `»» file_id`                          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                               | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» queue_position`                   | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» queue_size`                       | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» started_at`                       | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» status`                           | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»» tags`                             | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» worker_id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» max_deadline`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» reason`                            | [codersdk.BuildReason](schemas.md#codersdkbuildreason)                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» resources`                         | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» agents`                           | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» apps`                            | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»»» command`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» display_name`                   | string                                                                                                 | false    |              | »»»display name is a friendly name for the app.                                                                                                                                                                                                |
| `»»»» external`                       | boolean                                                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»»» health`                         | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» healthcheck`                    | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                                                 | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»»» interval`                      | integer                                                                                                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»»» threshold`                     | integer                                                                                                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»»» url`                           | string                                                                                                 | false    |              | »»»»url specifies the endpoint to check for the app health.                                                                                                                                                                                    |
| `»»»» icon`                           | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»»» id`                             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» sharing_level`                  | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»»» slug`                           | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»»» subdomain`                      | boolean                                                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»»» url`                            | string                                                                                                 | false    |              | »»»url is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                      |
| `»»» architecture`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» connection_timeout_seconds`      | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» created_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» directory`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» disconnected_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_apps`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» environment_variables`           | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» [any property]`                 | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» expanded_directory`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» first_connected_at`              | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» health`                          | [codersdk.WorkspaceAgentHealth](schemas.md#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                                                                                        |
| `»»»» healthy`                        | boolean                                                                                                | false    |              | Healthy is true if the agent is healthy.                                                                                                                                                                                                       |
| `»»»» reason`                         | string                                                                                                 | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.                                                                                                                                                  |
| `»»» id`                              | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» instance_id`                     | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» last_connected_at`               | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» latency`                         | object                                                                                                 | false    |              | »»latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                          |
| `»»»» [any property]`                 | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» latency_ms`                    | number                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» preferred`                     | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» lifecycle_state`                 | [codersdk.WorkspaceAgentLifecycle](schemas.md#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                                                                                                |
| `»»» login_before_ready`              | boolean                                                                                                | false    |              | Deprecated: Use StartupScriptBehavior instead.                                                                                                                                                                                                 |
| `»»» logs_length`                     | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» logs_overflowed`                 | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» name`                            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» operating_system`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» ready_at`                        | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» resource_id`                     | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» shutdown_script`                 | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» shutdown_script_timeout_seconds` | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» started_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» startup_script`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» startup_script_behavior`         | [codersdk.WorkspaceAgentStartupScriptBehavior](schemas.md#codersdkworkspaceagentstartupscriptbehavior) | false    |              |                                                                                                                                                                                                                                                |
| `»»» startup_script_timeout_seconds`  | integer                                                                                                | false    |              | »»startup script timeout seconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout.                                   |
| `»»» status`                          | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»»» subsystems`                      | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» troubleshooting_url`             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» updated_at`                      | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» version`                         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                       | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» daily_cost`                       | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» hide`                             | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» icon`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                               | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» job_id`                           | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» metadata`                         | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» key`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» sensitive`                       | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» value`                           | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» type`                             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» workspace_transition`             | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |
| `» status`                            | [codersdk.WorkspaceStatus](schemas.md#codersdkworkspacestatus)                                         | false    |              |                                                                                                                                                                                                                                                |
| `» template_version_id`               | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» template_version_name`             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» transition`                        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |
| `» updated_at`                        | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_id`                      | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_name`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_owner_id`                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_owner_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property                  | Value                         |
| ------------------------- | ----------------------------- |
| `error_code`              | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`                  | `pending`                     |
| `status`                  | `running`                     |
| `status`                  | `succeeded`                   |
| `status`                  | `canceling`                   |
| `status`                  | `canceled`                    |
| `status`                  | `failed`                      |
| `reason`                  | `initiator`                   |
| `reason`                  | `autostart`                   |
| `reason`                  | `autostop`                    |
| `health`                  | `disabled`                    |
| `health`                  | `initializing`                |
| `health`                  | `healthy`                     |
| `health`                  | `unhealthy`                   |
| `sharing_level`           | `owner`                       |
| `sharing_level`           | `authenticated`               |
| `sharing_level`           | `public`                      |
| `lifecycle_state`         | `created`                     |
| `lifecycle_state`         | `starting`                    |
| `lifecycle_state`         | `start_timeout`               |
| `lifecycle_state`         | `start_error`                 |
| `lifecycle_state`         | `ready`                       |
| `lifecycle_state`         | `shutting_down`               |
| `lifecycle_state`         | `shutdown_timeout`            |
| `lifecycle_state`         | `shutdown_error`              |
| `lifecycle_state`         | `off`                         |
| `startup_script_behavior` | `blocking`                    |
| `startup_script_behavior` | `non-blocking`                |
| `status`                  | `connecting`                  |
| `status`                  | `connected`                   |
| `status`                  | `disconnected`                |
| `status`                  | `timeout`                     |
| `workspace_transition`    | `start`                       |
| `workspace_transition`    | `stop`                        |
| `workspace_transition`    | `delete`                      |
| `status`                  | `pending`                     |
| `status`                  | `starting`                    |
| `status`                  | `running`                     |
| `status`                  | `stopping`                    |
| `status`                  | `stopped`                     |
| `status`                  | `failed`                      |
| `status`                  | `canceling`                   |
| `status`                  | `canceled`                    |
| `status`                  | `deleting`                    |
| `status`                  | `deleted`                     |
| `transition`              | `start`                       |
| `transition`              | `stop`                        |
| `transition`              | `delete`                      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create workspace build

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaces/{workspace}/builds \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaces/{workspace}/builds`

> Body parameter

```json
{
  "dry_run": true,
  "log_level": "debug",
  "orphan": true,
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "state": [0],
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "transition": "create"
}
```

### Parameters

| Name        | In   | Type                                                                                   | Required | Description                    |
| ----------- | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------------ |
| `workspace` | path | string(uuid)                                                                           | true     | Workspace ID                   |
| `body`      | body | [codersdk.CreateWorkspaceBuildRequest](schemas.md#codersdkcreateworkspacebuildrequest) | true     | Create workspace build request |

### Example responses

> 200 Response

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
  "initiator_name": "string",
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
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
          "login_before_ready": true,
          "logs_length": 0,
          "logs_overflowed": true,
          "name": "string",
          "operating_system": "string",
          "ready_at": "2019-08-24T14:15:22Z",
          "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
          "shutdown_script": "string",
          "shutdown_script_timeout_seconds": 0,
          "started_at": "2019-08-24T14:15:22Z",
          "startup_script": "string",
          "startup_script_behavior": "blocking",
          "startup_script_timeout_seconds": 0,
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

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
