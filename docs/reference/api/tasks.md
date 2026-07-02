# Tasks

## List AI tasks

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/tasks \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/tasks`

### Parameters

| Name | In    | Type   | Required | Description                                                                                                         |
|------|-------|--------|----------|---------------------------------------------------------------------------------------------------------------------|
| `q`  | query | string | false    | Search query for filtering tasks. Supports: owner:<username/uuid/me>, organization:<org-name/uuid>, status:<status> |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "tasks": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "current_state": {
        "message": "string",
        "state": "working",
        "timestamp": "2019-08-24T14:15:22Z",
        "uri": "string"
      },
      "display_name": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "initial_prompt": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_avatar_url": "string",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "status": "pending",
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "updated_at": "2019-08-24T14:15:22Z",
      "workspace_agent_health": {
        "healthy": false,
        "reason": "agent has lost connection"
      },
      "workspace_agent_id": {
        "uuid": "string",
        "valid": true
      },
      "workspace_agent_lifecycle": "created",
      "workspace_app_id": {
        "uuid": "string",
        "valid": true
      },
      "workspace_build_number": 0,
      "workspace_id": {
        "uuid": "string",
        "valid": true
      },
      "workspace_name": "string",
      "workspace_status": "pending"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TasksListResponse](schemas.md#codersdktaskslistresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a new AI task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/tasks/{user} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/tasks/{user}`

> Body parameter

```json
{
  "display_name": "string",
  "input": "string",
  "name": "string",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "template_version_preset_id": "512a53a7-30da-446e-a1fc-713c630baff1"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description                                           |
|--------|------|--------------------------------------------------------------------|----------|-------------------------------------------------------|
| `user` | path | string                                                             | true     | Username, user ID, or 'me' for the authenticated user |
| `body` | body | [codersdk.CreateTaskRequest](schemas.md#codersdkcreatetaskrequest) | true     | Create task request                                   |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "current_state": {
    "message": "string",
    "state": "working",
    "timestamp": "2019-08-24T14:15:22Z",
    "uri": "string"
  },
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initial_prompt": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "status": "pending",
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_agent_health": {
    "healthy": false,
    "reason": "agent has lost connection"
  },
  "workspace_agent_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_agent_lifecycle": "created",
  "workspace_app_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_build_number": 0,
  "workspace_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_name": "string",
  "workspace_status": "pending"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Task](schemas.md#codersdktask) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get AI task by ID or name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/tasks/{user}/{task} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/tasks/{user}/{task}`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "current_state": {
    "message": "string",
    "state": "working",
    "timestamp": "2019-08-24T14:15:22Z",
    "uri": "string"
  },
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initial_prompt": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "status": "pending",
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_agent_health": {
    "healthy": false,
    "reason": "agent has lost connection"
  },
  "workspace_agent_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_agent_lifecycle": "created",
  "workspace_app_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_build_number": 0,
  "workspace_id": {
    "uuid": "string",
    "valid": true
  },
  "workspace_name": "string",
  "workspace_status": "pending"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Task](schemas.md#codersdktask) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete AI task

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/tasks/{user}/{task} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/tasks/{user}/{task}`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Responses

| Status | Meaning                                                       | Description | Schema |
|--------|---------------------------------------------------------------|-------------|--------|
| 202    | [Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3) | Accepted    |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update AI task input

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/tasks/{user}/{task}/input \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/tasks/{user}/{task}/input`

> Body parameter

```json
{
  "input": "string"
}
```

### Parameters

| Name   | In   | Type                                                                         | Required | Description                                           |
|--------|------|------------------------------------------------------------------------------|----------|-------------------------------------------------------|
| `user` | path | string                                                                       | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string                                                                       | true     | Task ID, or task name                                 |
| `body` | body | [codersdk.UpdateTaskInputRequest](schemas.md#codersdkupdatetaskinputrequest) | true     | Update task input request                             |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get AI task logs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/tasks/{user}/{task}/logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/tasks/{user}/{task}/logs`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Example responses

> 200 Response

```json
{
  "logs": [
    {
      "content": "string",
      "id": 0,
      "time": "2019-08-24T14:15:22Z",
      "type": "input"
    }
  ],
  "snapshot": true,
  "snapshot_at": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TaskLogsResponse](schemas.md#codersdktasklogsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Pause task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/tasks/{user}/{task}/pause \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/tasks/{user}/{task}/pause`

### Parameters

| Name   | In   | Type         | Required | Description                                           |
|--------|------|--------------|----------|-------------------------------------------------------|
| `user` | path | string       | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string(uuid) | true     | Task ID                                               |

### Example responses

> 202 Response

```json
{
  "workspace_build": {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": "2019-08-24T14:15:22Z",
    "has_ai_task": true,
    "has_external_agent": true,
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
      "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
      "input": {
        "error": "string",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
      },
      "logs_overflowed": true,
      "metadata": {
        "template_display_name": "string",
        "template_icon": "string",
        "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
        "template_name": "string",
        "template_version_name": "string",
        "workspace_build_transition": "start",
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
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b",
      "worker_name": "string"
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
                "group": "string",
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
                "tooltip": "string",
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
            "parent_id": {
              "uuid": "string",
              "valid": true
            },
            "ready_at": "2019-08-24T14:15:22Z",
            "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
            "scripts": [
              {
                "cron": "string",
                "display_name": "string",
                "exit_code": 0,
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "log_path": "string",
                "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
                "run_on_start": true,
                "run_on_stop": true,
                "script": "string",
                "start_blocks_login": true,
                "status": "ok",
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
    "template_version_preset_id": "512a53a7-30da-446e-a1fc-713c630baff1",
    "transition": "start",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string",
    "workspace_owner_avatar_url": "string",
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  }
}
```

### Responses

| Status | Meaning                                                       | Description | Schema                                                             |
|--------|---------------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 202    | [Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3) | Accepted    | [codersdk.PauseTaskResponse](schemas.md#codersdkpausetaskresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Resume task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/tasks/{user}/{task}/resume \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/tasks/{user}/{task}/resume`

### Parameters

| Name   | In   | Type         | Required | Description                                           |
|--------|------|--------------|----------|-------------------------------------------------------|
| `user` | path | string       | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string(uuid) | true     | Task ID                                               |

### Example responses

> 202 Response

```json
{
  "workspace_build": {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": "2019-08-24T14:15:22Z",
    "has_ai_task": true,
    "has_external_agent": true,
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
      "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
      "input": {
        "error": "string",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
      },
      "logs_overflowed": true,
      "metadata": {
        "template_display_name": "string",
        "template_icon": "string",
        "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
        "template_name": "string",
        "template_version_name": "string",
        "workspace_build_transition": "start",
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
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b",
      "worker_name": "string"
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
                "group": "string",
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
                "tooltip": "string",
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
            "parent_id": {
              "uuid": "string",
              "valid": true
            },
            "ready_at": "2019-08-24T14:15:22Z",
            "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
            "scripts": [
              {
                "cron": "string",
                "display_name": "string",
                "exit_code": 0,
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "log_path": "string",
                "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
                "run_on_start": true,
                "run_on_stop": true,
                "script": "string",
                "start_blocks_login": true,
                "status": "ok",
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
    "template_version_preset_id": "512a53a7-30da-446e-a1fc-713c630baff1",
    "transition": "start",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string",
    "workspace_owner_avatar_url": "string",
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  }
}
```

### Responses

| Status | Meaning                                                       | Description | Schema                                                               |
|--------|---------------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 202    | [Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3) | Accepted    | [codersdk.ResumeTaskResponse](schemas.md#codersdkresumetaskresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send input to AI task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/tasks/{user}/{task}/send \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/tasks/{user}/{task}/send`

> Body parameter

```json
{
  "input": "string"
}
```

### Parameters

| Name   | In   | Type                                                           | Required | Description                                           |
|--------|------|----------------------------------------------------------------|----------|-------------------------------------------------------|
| `user` | path | string                                                         | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string                                                         | true     | Task ID, or task name                                 |
| `body` | body | [codersdk.TaskSendRequest](schemas.md#codersdktasksendrequest) | true     | Task input request                                    |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upload task log snapshot

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/me/tasks/{task}/log-snapshot?format=agentapi \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/workspaceagents/me/tasks/{task}/log-snapshot`

> Body parameter

```json
{}
```

### Parameters

| Name     | In    | Type         | Required | Description                                                  |
|----------|-------|--------------|----------|--------------------------------------------------------------|
| `task`   | path  | string(uuid) | true     | Task ID                                                      |
| `format` | query | string       | true     | Snapshot format                                              |
| `body`   | body  | object       | true     | Raw snapshot payload (structure depends on format parameter) |

#### Enumerated Values

| Parameter | Value(s)   |
|-----------|------------|
| `format`  | `agentapi` |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
