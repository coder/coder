# Tasks

## List AI tasks

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/tasks \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /tasks`

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

`POST /tasks/{user}`

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

`GET /tasks/{user}/{task}`

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

`DELETE /tasks/{user}/{task}`

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

`PATCH /tasks/{user}/{task}/input`

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

`GET /tasks/{user}/{task}/logs`

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
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TaskLogsResponse](schemas.md#codersdktasklogsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send input to AI task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/tasks/{user}/{task}/send \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /tasks/{user}/{task}/send`

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
