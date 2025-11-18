# Experimental

## List AI tasks

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/api/experimental/tasks \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/tasks`

### Parameters

| Name | In    | Type   | Required | Description                                                                                                         |
|------|-------|--------|----------|---------------------------------------------------------------------------------------------------------------------|
| `q`  | query | string | false    | Search query for filtering tasks. Supports: owner:<username/uuid/me>, organization:<org-name/uuid>, status:<status> |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TasksListResponse](schemas.md#codersdktaskslistresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a new AI task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/api/experimental/tasks/{user} \
  -H 'Content-Type: application/json' \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/tasks/{user}`

> Body parameter

```json
{
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

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Task](schemas.md#codersdktask) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get AI task by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{task} \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/tasks/{user}/{task}`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Task](schemas.md#codersdktask) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete AI task by ID

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{task} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/tasks/{user}/{task}`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Responses

| Status | Meaning                                                       | Description             | Schema |
|--------|---------------------------------------------------------------|-------------------------|--------|
| 202    | [Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3) | Task deletion initiated |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update AI task input

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{task}/input \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/experimental/tasks/{user}/{task}/input`

> Body parameter

```json
{
  "prompt": "string"
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
curl -X GET http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{task}/logs \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/tasks/{user}/{task}/logs`

### Parameters

| Name   | In   | Type   | Required | Description                                           |
|--------|------|--------|----------|-------------------------------------------------------|
| `user` | path | string | true     | Username, user ID, or 'me' for the authenticated user |
| `task` | path | string | true     | Task ID, or task name                                 |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TaskLogsResponse](schemas.md#codersdktasklogsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send input to AI task

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{task}/send \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/tasks/{user}/{task}/send`

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

| Status | Meaning                                                         | Description             | Schema |
|--------|-----------------------------------------------------------------|-------------------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | Input sent successfully |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
