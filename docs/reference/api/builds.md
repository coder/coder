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
|-----------------|------|----------------|----------|----------------------|
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
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
              "hidden": true,
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "open_in": "slim-window",
              "sharing_level": "owner",
              "slug": "string",
              "statuses": [
                {
                  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                  "created_at": "2019-08-24T14:15:22Z",
                  "icon": "string",
                  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                  "message": "string",
                  "needs_user_attention": true,
                  "state": "working",
                  "uri": "string",
                  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                }
              ],
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
          "display_apps": [
            "vscode"
          ],
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
              "display_name": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
          "subsystems": [
            "envbox"
          ],
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
  "workspace_owner_avatar_url": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
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
|------------------|------|--------|----------|--------------------|
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
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
              "hidden": true,
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "open_in": "slim-window",
              "sharing_level": "owner",
              "slug": "string",
              "statuses": [
                {
                  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                  "created_at": "2019-08-24T14:15:22Z",
                  "icon": "string",
                  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                  "message": "string",
                  "needs_user_attention": true,
                  "state": "working",
                  "uri": "string",
                  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                }
              ],
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
          "display_apps": [
            "vscode"
          ],
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
              "display_name": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
          "subsystems": [
            "envbox"
          ],
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
  "workspace_owner_avatar_url": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
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
|------------------|------|--------|----------|--------------------|
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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
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

| Name             | In    | Type    | Required | Description        |
|------------------|-------|---------|----------|--------------------|
| `workspacebuild` | path  | string  | true     | Workspace build ID |
| `before`         | query | integer | false    | Before log id      |
| `after`          | query | integer | false    | After log id       |
| `follow`         | query | boolean | false    | Follow log stream  |

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
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerJobLog](schemas.md#codersdkprovisionerjoblog) |

<h3 id="get-workspace-build-logs-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type                                               | Required | Restrictions | Description |
|----------------|----------------------------------------------------|----------|--------------|-------------|
| `[array item]` | array                                              | false    |              |             |
| `» created_at` | string(date-time)                                  | false    |              |             |
| `» id`         | integer                                            | false    |              |             |
| `» log_level`  | [codersdk.LogLevel](schemas.md#codersdkloglevel)   | false    |              |             |
| `» log_source` | [codersdk.LogSource](schemas.md#codersdklogsource) | false    |              |             |
| `» output`     | string                                             | false    |              |             |
| `» stage`      | string                                             | false    |              |             |

#### Enumerated Values

| Property     | Value                |
|--------------|----------------------|
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
|------------------|------|--------|----------|--------------------|
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
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceBuildParameter](schemas.md#codersdkworkspacebuildparameter) |

<h3 id="get-build-parameters-for-workspace-build-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `[array item]` | array  | false    |              |             |
| `» name`       | string | false    |              |             |
| `» value`      | string | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Removed: Get workspace resources for workspace build

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
|------------------|------|--------|----------|--------------------|
| `workspacebuild` | path | string | true     | Workspace build ID |

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
            "hidden": true,
            "icon": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "open_in": "slim-window",
            "sharing_level": "owner",
            "slug": "string",
            "statuses": [
              {
                "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                "created_at": "2019-08-24T14:15:22Z",
                "icon": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "message": "string",
                "needs_user_attention": true,
                "state": "working",
                "uri": "string",
                "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
              }
            ],
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
        "display_apps": [
          "vscode"
        ],
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
            "display_name": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
        "subsystems": [
          "envbox"
        ],
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
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceResource](schemas.md#codersdkworkspaceresource) |

<h3 id="removed:-get-workspace-resources-for-workspace-build-responseschema">Response Schema</h3>

Status Code **200**

| Name                            | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
|---------------------------------|--------------------------------------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
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
| `»»» hidden`                    | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» icon`                      | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» open_in`                   | [codersdk.WorkspaceAppOpenIn](schemas.md#codersdkworkspaceappopenin)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»» slug`                      | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»» statuses`                  | array                                                                                                  | false    |              | Statuses is a list of statuses for the app.                                                                                                                                                                                                    |
| `»»»» agent_id`                 | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» app_id`                   | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» created_at`               | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»»» icon`                     | string                                                                                                 | false    |              | Icon is an external URL to an icon that will be rendered in the UI.                                                                                                                                                                            |
| `»»»» id`                       | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» message`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» needs_user_attention`     | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»» state`                    | [codersdk.WorkspaceAppStatusState](schemas.md#codersdkworkspaceappstatusstate)                         | false    |              |                                                                                                                                                                                                                                                |
| `»»»» uri`                      | string                                                                                                 | false    |              | Uri is the URI of the resource that the status is for. e.g. https://github.com/org/repo/pull/123 e.g. file:///path/to/file                                                                                                                     |
| `»»»» workspace_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
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
| `»»» display_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
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
|---------------------------|--------------------|
| `health`                  | `disabled`         |
| `health`                  | `initializing`     |
| `health`                  | `healthy`          |
| `health`                  | `unhealthy`        |
| `open_in`                 | `slim-window`      |
| `open_in`                 | `tab`              |
| `sharing_level`           | `owner`            |
| `sharing_level`           | `authenticated`    |
| `sharing_level`           | `public`           |
| `state`                   | `working`          |
| `state`                   | `complete`         |
| `state`                   | `failure`          |
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
|------------------|------|--------|----------|--------------------|
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
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
              "hidden": true,
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "open_in": "slim-window",
              "sharing_level": "owner",
              "slug": "string",
              "statuses": [
                {
                  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                  "created_at": "2019-08-24T14:15:22Z",
                  "icon": "string",
                  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                  "message": "string",
                  "needs_user_attention": true,
                  "state": "working",
                  "uri": "string",
                  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                }
              ],
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
          "display_apps": [
            "vscode"
          ],
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
              "display_name": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
          "subsystems": [
            "envbox"
          ],
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
  "workspace_owner_avatar_url": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace build timings by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspacebuilds/{workspacebuild}/timings \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspacebuilds/{workspacebuild}/timings`

### Parameters

| Name             | In   | Type         | Required | Description        |
|------------------|------|--------------|----------|--------------------|
| `workspacebuild` | path | string(uuid) | true     | Workspace build ID |

### Example responses

> 200 Response

```json
{
  "agent_connection_timings": [
    {
      "ended_at": "2019-08-24T14:15:22Z",
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z",
      "workspace_agent_id": "string",
      "workspace_agent_name": "string"
    }
  ],
  "agent_script_timings": [
    {
      "display_name": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "exit_code": 0,
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "string",
      "workspace_agent_id": "string",
      "workspace_agent_name": "string"
    }
  ],
  "provisioner_timings": [
    {
      "action": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
      "resource": "string",
      "source": "string",
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuildTimings](schemas.md#codersdkworkspacebuildtimings) |

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
|-------------|-------|-------------------|----------|-----------------|
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
      "available_workers": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "error_code": "REQUIRED_TEMPLATE_VARIABLES",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "input": {
        "error": "string",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
      },
      "metadata": {
        "template_display_name": "string",
        "template_icon": "string",
        "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
        "template_name": "string",
        "template_version_name": "string",
        "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
        "workspace_name": "string"
      },
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "queue_position": 0,
      "queue_size": 0,
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "type": "template_version_import",
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "matched_provisioners": {
      "available": 0,
      "count": 0,
      "most_recently_seen": "2019-08-24T14:15:22Z"
    },
    "max_deadline": "2019-08-24T14:15:22Z",
    "reason": "initiator",
    "resources": [
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
                "hidden": true,
                "icon": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "open_in": "slim-window",
                "sharing_level": "owner",
                "slug": "string",
                "statuses": [
                  {
                    "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                    "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                    "created_at": "2019-08-24T14:15:22Z",
                    "icon": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "message": "string",
                    "needs_user_attention": true,
                    "state": "working",
                    "uri": "string",
                    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                  }
                ],
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
            "display_apps": [
              "vscode"
            ],
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
                "display_name": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
            "subsystems": [
              "envbox"
            ],
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
    "workspace_owner_avatar_url": "string",
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

<h3 id="get-workspace-builds-by-workspace-id-responseschema">Response Schema</h3>

Status Code **200**

| Name                             | Type                                                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
|----------------------------------|--------------------------------------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                   | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `» build_number`                 | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» created_at`                   | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» daily_cost`                   | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `» deadline`                     | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» id`                           | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» initiator_id`                 | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» initiator_name`               | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» job`                          | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob)                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» available_workers`           | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» canceled_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» completed_at`                | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                  | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» error`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» error_code`                  | [codersdk.JobErrorCode](schemas.md#codersdkjoberrorcode)                                               | false    |              |                                                                                                                                                                                                                                                |
| `»» file_id`                     | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» input`                       | [codersdk.ProvisionerJobInput](schemas.md#codersdkprovisionerjobinput)                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» error`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_version_id`        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» workspace_build_id`         | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» metadata`                    | [codersdk.ProvisionerJobMetadata](schemas.md#codersdkprovisionerjobmetadata)                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_display_name`      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_icon`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_id`                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» template_version_name`      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» workspace_id`               | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» workspace_name`             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» organization_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» queue_position`              | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» queue_size`                  | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» started_at`                  | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» status`                      | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»» tags`                        | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» [any property]`             | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» type`                        | [codersdk.ProvisionerJobType](schemas.md#codersdkprovisionerjobtype)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»» worker_id`                   | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» matched_provisioners`         | [codersdk.MatchedProvisioners](schemas.md#codersdkmatchedprovisioners)                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» available`                   | integer                                                                                                | false    |              | Available is the number of provisioner daemons that are available to take jobs. This may be less than the count if some provisioners are busy or have been stopped.                                                                            |
| `»» count`                       | integer                                                                                                | false    |              | Count is the number of provisioner daemons that matched the given tags. If the count is 0, it means no provisioner daemons matched the requested tags.                                                                                         |
| `»» most_recently_seen`          | string(date-time)                                                                                      | false    |              | Most recently seen is the most recently seen time of the set of matched provisioners. If no provisioners matched, this field will be null.                                                                                                     |
| `» max_deadline`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» reason`                       | [codersdk.BuildReason](schemas.md#codersdkbuildreason)                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» resources`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»» agents`                      | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» api_version`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» apps`                       | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»»» command`                   | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» display_name`              | string                                                                                                 | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `»»»» external`                  | boolean                                                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `»»»» health`                    | [codersdk.WorkspaceAppHealth](schemas.md#codersdkworkspaceapphealth)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» healthcheck`               | [codersdk.Healthcheck](schemas.md#codersdkhealthcheck)                                                 | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `»»»»» interval`                 | integer                                                                                                | false    |              | Interval specifies the seconds between each health check.                                                                                                                                                                                      |
| `»»»»» threshold`                | integer                                                                                                | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy".                                                                                                                                               |
| `»»»»» url`                      | string                                                                                                 | false    |              | URL specifies the endpoint to check for the app health.                                                                                                                                                                                        |
| `»»»» hidden`                    | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»» icon`                      | string                                                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `»»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» open_in`                   | [codersdk.WorkspaceAppOpenIn](schemas.md#codersdkworkspaceappopenin)                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»» sharing_level`             | [codersdk.WorkspaceAppSharingLevel](schemas.md#codersdkworkspaceappsharinglevel)                       | false    |              |                                                                                                                                                                                                                                                |
| `»»»» slug`                      | string                                                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `»»»» statuses`                  | array                                                                                                  | false    |              | Statuses is a list of statuses for the app.                                                                                                                                                                                                    |
| `»»»»» agent_id`                 | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» app_id`                   | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» created_at`               | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» icon`                     | string                                                                                                 | false    |              | Icon is an external URL to an icon that will be rendered in the UI.                                                                                                                                                                            |
| `»»»»» id`                       | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» message`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» needs_user_attention`     | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» state`                    | [codersdk.WorkspaceAppStatusState](schemas.md#codersdkworkspaceappstatusstate)                         | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» uri`                      | string                                                                                                 | false    |              | Uri is the URI of the resource that the status is for. e.g. https://github.com/org/repo/pull/123 e.g. file:///path/to/file                                                                                                                     |
| `»»»»» workspace_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» subdomain`                 | boolean                                                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `»»»» subdomain_name`            | string                                                                                                 | false    |              | Subdomain name is the application domain exposed on the `coder server`.                                                                                                                                                                        |
| `»»»» url`                       | string                                                                                                 | false    |              | URL is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |
| `»»» architecture`               | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» connection_timeout_seconds` | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» created_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» directory`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» disconnected_at`            | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» display_apps`               | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» environment_variables`      | object                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» [any property]`            | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» expanded_directory`         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» first_connected_at`         | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» health`                     | [codersdk.WorkspaceAgentHealth](schemas.md#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                                                                                        |
| `»»»» healthy`                   | boolean                                                                                                | false    |              | Healthy is true if the agent is healthy.                                                                                                                                                                                                       |
| `»»»» reason`                    | string                                                                                                 | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true.                                                                                                                                                  |
| `»»» id`                         | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» instance_id`                | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» last_connected_at`          | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» latency`                    | object                                                                                                 | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                                                            |
| `»»»» [any property]`            | [codersdk.DERPRegion](schemas.md#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» latency_ms`               | number                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»»» preferred`                | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» lifecycle_state`            | [codersdk.WorkspaceAgentLifecycle](schemas.md#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                                                                                                |
| `»»» log_sources`                | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»»» created_at`                | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»»» display_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» icon`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» workspace_agent_id`        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» logs_length`                | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» logs_overflowed`            | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» name`                       | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» operating_system`           | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» ready_at`                   | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» resource_id`                | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»» scripts`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»»» cron`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» display_name`              | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» id`                        | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» log_path`                  | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» log_source_id`             | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»»»» run_on_start`              | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»» run_on_stop`               | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»» script`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»»» start_blocks_login`        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»»» timeout`                   | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» started_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» startup_script_behavior`    | [codersdk.WorkspaceAgentStartupScriptBehavior](schemas.md#codersdkworkspaceagentstartupscriptbehavior) | false    |              | Startup script behavior is a legacy field that is deprecated in favor of the `coder_script` resource. It's only referenced by old clients. Deprecated: Remove in the future!                                                                   |
| `»»» status`                     | [codersdk.WorkspaceAgentStatus](schemas.md#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                                                                                                |
| `»»» subsystems`                 | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» troubleshooting_url`        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» updated_at`                 | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»»» version`                    | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» created_at`                  | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `»» daily_cost`                  | integer                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» hide`                        | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»» icon`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» id`                          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» job_id`                      | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `»» metadata`                    | array                                                                                                  | false    |              |                                                                                                                                                                                                                                                |
| `»»» key`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»»» sensitive`                  | boolean                                                                                                | false    |              |                                                                                                                                                                                                                                                |
| `»»» value`                      | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» name`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» type`                        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `»» workspace_transition`        | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |
| `» status`                       | [codersdk.WorkspaceStatus](schemas.md#codersdkworkspacestatus)                                         | false    |              |                                                                                                                                                                                                                                                |
| `» template_version_id`          | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» template_version_name`        | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» transition`                   | [codersdk.WorkspaceTransition](schemas.md#codersdkworkspacetransition)                                 | false    |              |                                                                                                                                                                                                                                                |
| `» updated_at`                   | string(date-time)                                                                                      | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_id`                 | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_name`               | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_owner_avatar_url`   | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_owner_id`           | string(uuid)                                                                                           | false    |              |                                                                                                                                                                                                                                                |
| `» workspace_owner_name`         | string                                                                                                 | false    |              |                                                                                                                                                                                                                                                |

#### Enumerated Values

| Property                  | Value                         |
|---------------------------|-------------------------------|
| `error_code`              | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`                  | `pending`                     |
| `status`                  | `running`                     |
| `status`                  | `succeeded`                   |
| `status`                  | `canceling`                   |
| `status`                  | `canceled`                    |
| `status`                  | `failed`                      |
| `type`                    | `template_version_import`     |
| `type`                    | `workspace_build`             |
| `type`                    | `template_version_dry_run`    |
| `reason`                  | `initiator`                   |
| `reason`                  | `autostart`                   |
| `reason`                  | `autostop`                    |
| `health`                  | `disabled`                    |
| `health`                  | `initializing`                |
| `health`                  | `healthy`                     |
| `health`                  | `unhealthy`                   |
| `open_in`                 | `slim-window`                 |
| `open_in`                 | `tab`                         |
| `sharing_level`           | `owner`                       |
| `sharing_level`           | `authenticated`               |
| `sharing_level`           | `public`                      |
| `state`                   | `working`                     |
| `state`                   | `complete`                    |
| `state`                   | `failure`                     |
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
  "state": [
    0
  ],
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "transition": "start"
}
```

### Parameters

| Name        | In   | Type                                                                                   | Required | Description                    |
|-------------|------|----------------------------------------------------------------------------------------|----------|--------------------------------|
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
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
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
              "hidden": true,
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "open_in": "slim-window",
              "sharing_level": "owner",
              "slug": "string",
              "statuses": [
                {
                  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                  "created_at": "2019-08-24T14:15:22Z",
                  "icon": "string",
                  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                  "message": "string",
                  "needs_user_attention": true,
                  "state": "working",
                  "uri": "string",
                  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                }
              ],
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
          "display_apps": [
            "vscode"
          ],
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
              "display_name": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
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
          "subsystems": [
            "envbox"
          ],
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
  "workspace_owner_avatar_url": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceBuild](schemas.md#codersdkworkspacebuild) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
