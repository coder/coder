# User-Secrets

## List user secrets

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/secrets \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/secrets`

### Example responses

> 200 Response

```json
{
  "secrets": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "description": "string",
      "env_name": "string",
      "file_path": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "name": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                         |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ListUserSecretsResponse](schemas.md#codersdklistusersecretsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create user secret

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/secrets \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/secrets`

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

| Name   | In   | Type                                                                           | Required | Description  |
|--------|------|--------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.CreateUserSecretRequest](schemas.md#codersdkcreateusersecretrequest) | true     | Request body |

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
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserSecret](schemas.md#codersdkusersecret) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user secret

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/secrets/{name} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/secrets/{name}`

### Parameters

| Name   | In   | Type           | Required | Description |
|--------|------|----------------|----------|-------------|
| `name` | path | string(string) | true     | name        |

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
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                               |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserSecret](schemas.md#codersdkusersecret) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user secret value

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/secrets/{name}/value \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/secrets/{name}/value`

### Parameters

| Name   | In   | Type           | Required | Description |
|--------|------|----------------|----------|-------------|
| `name` | path | string(string) | true     | name        |

### Example responses

> 200 Response

```json
{
  "value": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserSecretValue](schemas.md#codersdkusersecretvalue) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
