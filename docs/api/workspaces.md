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

| Name      | In    | Type   | Required | Description                                 |
| --------- | ----- | ------ | -------- | ------------------------------------------- |
| owner     | query | string | false    | Filter by owner username                    |
| template  | query | string | false    | Filter by template name                     |
| name      | query | string | false    | Filter with partial-match by workspace name |
| status    | query | string | false    | Filter by workspace status                  |
| has_agent | query | string | false    | Filter by agent status                      |

#### Enumerated Values

| Parameter | Value        |
| --------- | ------------ |
| status    | pending      |
| status    | running      |
| status    | stopping     |
| status    | stopped      |
| status    | failed       |
| status    | canceling    |
| status    | canceled     |
| status    | deleted      |
| status    | deleting     |
| has_agent | connected    |
| has_agent | connecting   |
| has_agent | disconnected |
| has_agent | timeout      |

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

| Status | Meaning                                                 | Description | Schema                                                               |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspacesResponse](schemas.md#codersdkworkspacesresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get workspace metadata by ID

### Code samples

```shell
# You can also use wget
curl -X GET http://coder-server:8080/api/v2/workspaces/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /workspaces/{id}`

### Parameters

| Name            | In    | Type         | Required | Description                                                 |
| --------------- | ----- | ------------ | -------- | ----------------------------------------------------------- |
| id              | path  | string(uuid) | true     | Workspace ID                                                |
| include_deleted | query | string       | false    | Return data instead of HTTP 404 if the workspace is deleted |

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

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
