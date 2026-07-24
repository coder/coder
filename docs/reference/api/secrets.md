# Secrets

## List user secrets

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/secrets \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/users/{user}/secrets`

### Parameters

| Name   | In   | Type   | Required | Description              |
|--------|------|--------|----------|--------------------------|
| `user` | path | string | true     | User ID, username, or me |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "description": "string",
    "env_name": "string",
    "file_path": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserSecret](schemas.md#codersdkusersecret) |

<h3 id="list-user-secrets-responseschema">Response Schema</h3>

Status Code **200**

| Name            | Type              | Required | Restrictions | Description |
|-----------------|-------------------|----------|--------------|-------------|
| `[array item]`  | array             | false    |              |             |
| `» created_at`  | string(date-time) | false    |              |             |
| `» description` | string            | false    |              |             |
| `» env_name`    | string            | false    |              |             |
| `» file_path`   | string            | false    |              |             |
| `» id`          | string(uuid)      | false    |              |             |
| `» name`        | string            | false    |              |             |
| `» updated_at`  | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create a new user secret

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/secrets \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/users/{user}/secrets`

> Body parameter

```json
{
  "description": "string",
  "env_name": "string",
  "file_path": "string",
  "name": "string",
  "value": "string"
}
```

### Parameters

| Name   | In   | Type                                                                           | Required | Description              |
|--------|------|--------------------------------------------------------------------------------|----------|--------------------------|
| `user` | path | string                                                                         | true     | User ID, username, or me |
| `body` | body | [codersdk.CreateUserSecretRequest](schemas.md#codersdkcreateusersecretrequest) | true     | Create secret request    |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "env_name": "string",
  "file_path": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                               |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.UserSecret](schemas.md#codersdkusersecret) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Import user secrets from a file

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/secrets/batch \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/users/{user}/secrets/batch`

> Body parameter

```json
{
  "content": "string",
  "format": "env"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description              |
|--------|------|----------------------------------------------------------------------------------|----------|--------------------------|
| `user` | path | string                                                                           | true     | User ID, username, or me |
| `body` | body | [codersdk.ImportUserSecretsRequest](schemas.md#codersdkimportusersecretsrequest) | true     | Import secrets request   |

### Example responses

> 201 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "description": "string",
    "env_name": "string",
    "file_path": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                                 | Description              | Schema                                                        |
|--------|-------------------------------------------------------------------------|--------------------------|---------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)            | Created                  | array of [codersdk.UserSecret](schemas.md#codersdkusersecret) |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)        | Bad Request              | [codersdk.Response](schemas.md#codersdkresponse)              |
| 409    | [Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)           | Conflict                 | [codersdk.Response](schemas.md#codersdkresponse)              |
| 413    | [Payload Too Large](https://tools.ietf.org/html/rfc7231#section-6.5.11) | Request Entity Too Large | [codersdk.Response](schemas.md#codersdkresponse)              |

<h3 id="import-user-secrets-from-a-file-responseschema">Response Schema</h3>

Status Code **201**

| Name            | Type              | Required | Restrictions | Description |
|-----------------|-------------------|----------|--------------|-------------|
| `[array item]`  | array             | false    |              |             |
| `» created_at`  | string(date-time) | false    |              |             |
| `» description` | string            | false    |              |             |
| `» env_name`    | string            | false    |              |             |
| `» file_path`   | string            | false    |              |             |
| `» id`          | string(uuid)      | false    |              |             |
| `» name`        | string            | false    |              |             |
| `» updated_at`  | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get a user secret by name

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/secrets/{name} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/users/{user}/secrets/{name}`

### Parameters

| Name   | In   | Type   | Required | Description              |
|--------|------|--------|----------|--------------------------|
| `user` | path | string | true     | User ID, username, or me |
| `name` | path | string | true     | Secret name              |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "env_name": "string",
  "file_path": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserSecret](schemas.md#codersdkusersecret) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete a user secret

### Code samples

```sh
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/users/{user}/secrets/{name} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/users/{user}/secrets/{name}`

### Parameters

| Name   | In   | Type   | Required | Description              |
|--------|------|--------|----------|--------------------------|
| `user` | path | string | true     | User ID, username, or me |
| `name` | path | string | true     | Secret name              |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update a user secret

### Code samples

```sh
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/users/{user}/secrets/{name} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/users/{user}/secrets/{name}`

> Body parameter

```json
{
  "description": "string",
  "env_name": "string",
  "file_path": "string",
  "value": "string"
}
```

### Parameters

| Name   | In   | Type                                                                           | Required | Description              |
|--------|------|--------------------------------------------------------------------------------|----------|--------------------------|
| `user` | path | string                                                                         | true     | User ID, username, or me |
| `name` | path | string                                                                         | true     | Secret name              |
| `body` | body | [codersdk.UpdateUserSecretRequest](schemas.md#codersdkupdateusersecretrequest) | true     | Update secret request    |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "env_name": "string",
  "file_path": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserSecret](schemas.md#codersdkusersecret) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
