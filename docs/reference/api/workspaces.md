# Workspaces

## Create user workspace by organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/workspaces \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/members/{user}/workspaces`

Create a new workspace using a template. The request must
specify either the Template ID or the Template Version ID,
not both. If the Template ID is specified, the active version
of the template will be used.

> Body parameter

```json
{
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "name": "string",
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "ttl_ms": 0
}
```

### Parameters

| Name           | In   | Type                                                                         | Required | Description              |
|----------------|------|------------------------------------------------------------------------------|----------|--------------------------|
| `organization` | path | string(uuid)                                                                 | true     | Organization ID          |
| `user`         | path | string                                                                       | true     | Username, UUID, or me    |
| `body`         | body | [codersdk.CreateWorkspaceRequest](schemas.md#codersdkcreateworkspacerequest) | true     | Create workspace request |

### Example responses

> 200 Response

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
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
  },
  "latest_build": {
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
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace metadata by user and workspace name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/workspace/{workspacename} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/workspace/{workspacename}`

### Parameters

| Name              | In    | Type    | Required | Description                                                 |
|-------------------|-------|---------|----------|-------------------------------------------------------------|
| `user`            | path  | string  | true     | User ID, name, or me                                        |
| `workspacename`   | path  | string  | true     | Workspace name                                              |
| `include_deleted` | query | boolean | false    | Return data instead of HTTP 404 if the workspace is deleted |

### Example responses

> 200 Response

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
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
  },
  "latest_build": {
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
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create user workspace

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/workspaces \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/workspaces`

Create a new workspace using a template. The request must
specify either the Template ID or the Template Version ID,
not both. If the Template ID is specified, the active version
of the template will be used.

> Body parameter

```json
{
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "name": "string",
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "ttl_ms": 0
}
```

### Parameters

| Name   | In   | Type                                                                         | Required | Description              |
|--------|------|------------------------------------------------------------------------------|----------|--------------------------|
| `user` | path | string                                                                       | true     | Username, UUID, or me    |
| `body` | body | [codersdk.CreateWorkspaceRequest](schemas.md#codersdkcreateworkspacerequest) | true     | Create workspace request |

### Example responses

> 200 Response

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
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
  },
  "latest_build": {
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
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List workspaces

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces`

### Parameters

| Name     | In    | Type    | Required | Description                                                                                                                                       |
|----------|-------|---------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `q`      | query | string  | false    | Search query in the format `key:value`. Available keys are: owner, template, name, status, has-agent, dormant, last_used_after, last_used_before. |
| `limit`  | query | integer | false    | Page limit                                                                                                                                        |
| `offset` | query | integer | false    | Page offset                                                                                                                                       |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "workspaces": [
    {
      "allow_renames": true,
      "automatic_updates": "always",
      "autostart_schedule": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "deleting_at": "2019-08-24T14:15:22Z",
      "dormant_at": "2019-08-24T14:15:22Z",
      "favorite": true,
      "health": {
        "failing_agents": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "healthy": false
      },
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_used_at": "2019-08-24T14:15:22Z",
      "latest_app_status": {
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
      },
      "latest_build": {
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
                    "healthcheck": {},
                    "hidden": true,
                    "icon": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "open_in": "slim-window",
                    "sharing_level": "owner",
                    "slug": "string",
                    "statuses": [],
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
      },
      "name": "string",
      "next_start_at": "2019-08-24T14:15:22Z",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "organization_name": "string",
      "outdated": true,
      "owner_avatar_url": "string",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_require_active_version": true,
      "ttl_ms": 0,
      "updated_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspacesResponse](schemas.md#codersdkworkspacesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace metadata by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}`

### Parameters

| Name              | In    | Type         | Required | Description                                                 |
|-------------------|-------|--------------|----------|-------------------------------------------------------------|
| `workspace`       | path  | string(uuid) | true     | Workspace ID                                                |
| `include_deleted` | query | boolean      | false    | Return data instead of HTTP 404 if the workspace is deleted |

### Example responses

> 200 Response

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
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
  },
  "latest_build": {
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
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update workspace metadata by ID

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/workspaces/{workspace} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /workspaces/{workspace}`

> Body parameter

```json
{
  "name": "string"
}
```

### Parameters

| Name        | In   | Type                                                                         | Required | Description             |
|-------------|------|------------------------------------------------------------------------------|----------|-------------------------|
| `workspace` | path | string(uuid)                                                                 | true     | Workspace ID            |
| `body`      | body | [codersdk.UpdateWorkspaceRequest](schemas.md#codersdkupdateworkspacerequest) | true     | Metadata update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update workspace autostart schedule by ID

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/autostart \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/autostart`

> Body parameter

```json
{
  "schedule": "string"
}
```

### Parameters

| Name        | In   | Type                                                                                           | Required | Description             |
|-------------|------|------------------------------------------------------------------------------------------------|----------|-------------------------|
| `workspace` | path | string(uuid)                                                                                   | true     | Workspace ID            |
| `body`      | body | [codersdk.UpdateWorkspaceAutostartRequest](schemas.md#codersdkupdateworkspaceautostartrequest) | true     | Schedule update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update workspace automatic updates by ID

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/autoupdates \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/autoupdates`

> Body parameter

```json
{
  "automatic_updates": "always"
}
```

### Parameters

| Name        | In   | Type                                                                                                         | Required | Description               |
|-------------|------|--------------------------------------------------------------------------------------------------------------|----------|---------------------------|
| `workspace` | path | string(uuid)                                                                                                 | true     | Workspace ID              |
| `body`      | body | [codersdk.UpdateWorkspaceAutomaticUpdatesRequest](schemas.md#codersdkupdateworkspaceautomaticupdatesrequest) | true     | Automatic updates request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update workspace dormancy status by id

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/dormant \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/dormant`

> Body parameter

```json
{
  "dormant": true
}
```

### Parameters

| Name        | In   | Type                                                                           | Required | Description                        |
|-------------|------|--------------------------------------------------------------------------------|----------|------------------------------------|
| `workspace` | path | string(uuid)                                                                   | true     | Workspace ID                       |
| `body`      | body | [codersdk.UpdateWorkspaceDormancy](schemas.md#codersdkupdateworkspacedormancy) | true     | Make a workspace dormant or active |

### Example responses

> 200 Response

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
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
  },
  "latest_build": {
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
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Extend workspace deadline by ID

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/extend \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/extend`

> Body parameter

```json
{
  "deadline": "2019-08-24T14:15:22Z"
}
```

### Parameters

| Name        | In   | Type                                                                               | Required | Description                    |
|-------------|------|------------------------------------------------------------------------------------|----------|--------------------------------|
| `workspace` | path | string(uuid)                                                                       | true     | Workspace ID                   |
| `body`      | body | [codersdk.PutExtendWorkspaceRequest](schemas.md#codersdkputextendworkspacerequest) | true     | Extend deadline update request |

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

## Favorite workspace by ID

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/favorite \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/favorite`

### Parameters

| Name        | In   | Type         | Required | Description  |
|-------------|------|--------------|----------|--------------|
| `workspace` | path | string(uuid) | true     | Workspace ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Unfavorite workspace by ID

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/workspaces/{workspace}/favorite \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /workspaces/{workspace}/favorite`

### Parameters

| Name        | In   | Type         | Required | Description  |
|-------------|------|--------------|----------|--------------|
| `workspace` | path | string(uuid) | true     | Workspace ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Resolve workspace autostart by id

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace}/resolve-autostart \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}/resolve-autostart`

### Parameters

| Name        | In   | Type         | Required | Description  |
|-------------|------|--------------|----------|--------------|
| `workspace` | path | string(uuid) | true     | Workspace ID |

### Example responses

> 200 Response

```json
{
  "parameter_mismatch": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ResolveAutostartResponse](schemas.md#codersdkresolveautostartresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace timings by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace}/timings \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}/timings`

### Parameters

| Name        | In   | Type         | Required | Description  |
|-------------|------|--------------|----------|--------------|
| `workspace` | path | string(uuid) | true     | Workspace ID |

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

## Update workspace TTL by ID

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/workspaces/{workspace}/ttl \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /workspaces/{workspace}/ttl`

> Body parameter

```json
{
  "ttl_ms": 0
}
```

### Parameters

| Name        | In   | Type                                                                               | Required | Description                  |
|-------------|------|------------------------------------------------------------------------------------|----------|------------------------------|
| `workspace` | path | string(uuid)                                                                       | true     | Workspace ID                 |
| `body`      | body | [codersdk.UpdateWorkspaceTTLRequest](schemas.md#codersdkupdateworkspacettlrequest) | true     | Workspace TTL update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Post Workspace Usage by ID

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaces/{workspace}/usage \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaces/{workspace}/usage`

> Body parameter

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "app_name": "vscode"
}
```

### Parameters

| Name        | In   | Type                                                                               | Required | Description                  |
|-------------|------|------------------------------------------------------------------------------------|----------|------------------------------|
| `workspace` | path | string(uuid)                                                                       | true     | Workspace ID                 |
| `body`      | body | [codersdk.PostWorkspaceUsageRequest](schemas.md#codersdkpostworkspaceusagerequest) | false    | Post workspace usage request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch workspace by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace}/watch \
  -H 'Accept: text/event-stream' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}/watch`

### Parameters

| Name        | In   | Type         | Required | Description  |
|-------------|------|--------------|----------|--------------|
| `workspace` | path | string(uuid) | true     | Workspace ID |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
