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

| Name       | In    | Type    | Required | Description                               |
|------------|-------|---------|----------|-------------------------------------------|
| `q`        | query | string  | false    | Search query for filtering tasks          |
| `after_id` | query | string  | false    | Return tasks after this ID for pagination |
| `limit`    | query | integer | false    | Maximum number of tasks to return         |
| `offset`   | query | integer | false    | Offset for pagination                     |

### Example responses

> 200 Response

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [coderd.tasksListResponse](schemas.md#coderdtaskslistresponse) |

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
curl -X GET http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{id} \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/tasks/{user}/{id}`

### Parameters

| Name   | In   | Type         | Required | Description                                           |
|--------|------|--------------|----------|-------------------------------------------------------|
| `user` | path | string       | true     | Username, user ID, or 'me' for the authenticated user |
| `id`   | path | string(uuid) | true     | Task ID                                               |

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
curl -X DELETE http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/tasks/{user}/{id}`

### Parameters

| Name   | In   | Type         | Required | Description                                           |
|--------|------|--------------|----------|-------------------------------------------------------|
| `user` | path | string       | true     | Username, user ID, or 'me' for the authenticated user |
| `id`   | path | string(uuid) | true     | Task ID                                               |

### Responses

| Status | Meaning                                                       | Description             | Schema |
|--------|---------------------------------------------------------------|-------------------------|--------|
| 202    | [Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3) | Task deletion initiated |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get AI task logs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{id}/logs \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/tasks/{user}/{id}/logs`

### Parameters

| Name   | In   | Type         | Required | Description                                           |
|--------|------|--------------|----------|-------------------------------------------------------|
| `user` | path | string       | true     | Username, user ID, or 'me' for the authenticated user |
| `id`   | path | string(uuid) | true     | Task ID                                               |

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
curl -X POST http://coder-server:8080/api/v2/api/experimental/tasks/{user}/{id}/send \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/tasks/{user}/{id}/send`

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
| `id`   | path | string(uuid)                                                   | true     | Task ID                                               |
| `body` | body | [codersdk.TaskSendRequest](schemas.md#codersdktasksendrequest) | true     | Task input request                                    |

### Responses

| Status | Meaning                                                         | Description             | Schema |
|--------|-----------------------------------------------------------------|-------------------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | Input sent successfully |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
