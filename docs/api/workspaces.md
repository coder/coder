# Workspaces

> This page is incomplete, stay tuned.

## Create workspace by organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/workspaces \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/members/{user}/workspaces`

### Parameters

| Name         | In   | Type         | Required | Description     |
| ------------ | ---- | ------------ | -------- | --------------- |
| organization | path | string(uuid) | true     | Organization ID |
| user         | path | string       | true     | Username        |

### Example responses

> 200 Response

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

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get workspace metadata by owner and workspace name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/workspace/{workspacename} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/workspace/{workspacename}`

### Parameters

| Name            | In    | Type    | Required | Description                                                 |
| --------------- | ----- | ------- | -------- | ----------------------------------------------------------- |
| user            | path  | string  | true     | Owner username                                              |
| workspacename   | path  | string  | true     | Workspace name                                              |
| include_deleted | query | boolean | false    | Return data instead of HTTP 404 if the workspace is deleted |

### Example responses

> 200 Response

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

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspacesResponse](schemas.md#codersdkworkspacesresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get workspace metadata by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{id}`

### Parameters

| Name            | In    | Type         | Required | Description                                                 |
| --------------- | ----- | ------------ | -------- | ----------------------------------------------------------- |
| id              | path  | string(uuid) | true     | Workspace ID                                                |
| include_deleted | query | boolean      | false    | Return data instead of HTTP 404 if the workspace is deleted |

### Example responses

> 200 Response

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

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Workspace](schemas.md#codersdkworkspace) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

| Name      | In   | Type                                                                         | Required | Description             |
| --------- | ---- | ---------------------------------------------------------------------------- | -------- | ----------------------- |
| workspace | path | string(uuid)                                                                 | true     | Workspace ID            |
| body      | body | [codersdk.UpdateWorkspaceRequest](schemas.md#codersdkupdateworkspacerequest) | true     | Metadata update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

| Name      | In   | Type                                                                                           | Required | Description             |
| --------- | ---- | ---------------------------------------------------------------------------------------------- | -------- | ----------------------- |
| workspace | path | string(uuid)                                                                                   | true     | Workspace ID            |
| body      | body | [codersdk.UpdateWorkspaceAutostartRequest](schemas.md#codersdkupdateworkspaceautostartrequest) | true     | Schedule update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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
  "deadline": "string"
}
```

### Parameters

| Name      | In   | Type                                                                               | Required | Description                    |
| --------- | ---- | ---------------------------------------------------------------------------------- | -------- | ------------------------------ |
| workspace | path | string(uuid)                                                                       | true     | Workspace ID                   |
| body      | body | [codersdk.PutExtendWorkspaceRequest](schemas.md#codersdkputextendworkspacerequest) | true     | Extend deadline update request |

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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

| Name      | In   | Type                                                                               | Required | Description                  |
| --------- | ---- | ---------------------------------------------------------------------------------- | -------- | ---------------------------- |
| workspace | path | string(uuid)                                                                       | true     | Workspace ID                 |
| body      | body | [codersdk.UpdateWorkspaceTTLRequest](schemas.md#codersdkupdateworkspacettlrequest) | true     | Workspace TTL update request |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

| Name      | In   | Type         | Required | Description  |
| --------- | ---- | ------------ | -------- | ------------ |
| workspace | path | string(uuid) | true     | Workspace ID |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
