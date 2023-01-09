# Users

> This page is incomplete, stay tuned.

## Get users

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users`

### Parameters

| Name       | In    | Type         | Required | Description  |
| ---------- | ----- | ------------ | -------- | ------------ |
| `q`        | query | string       | false    | Search query |
| `after_id` | query | string(uuid) | false    | After ID     |
| `limit`    | query | integer      | false    | Page limit   |
| `offset`   | query | integer      | false    | Page offset  |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "users": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GetUsersResponse](schemas.md#codersdkgetusersresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create new user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users`

> Body parameter

```json
{
  "email": "user@example.com",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "password": "string",
  "username": "string"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description         |
| ------ | ---- | ------------------------------------------------------------------ | -------- | ------------------- |
| `body` | body | [codersdk.CreateUserRequest](schemas.md#codersdkcreateuserrequest) | true     | Create user request |

### Example responses

> 201 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
  "roles": [
    {
      "display_name": "string",
      "name": "string"
    }
  ],
  "status": "active",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
| ------ | ------------------------------------------------------------ | ----------- | ---------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get authentication methods

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/authmethods \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/authmethods`

### Example responses

> 200 Response

```json
{
  "github": true,
  "oidc": true,
  "password": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                 |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AuthMethods](schemas.md#codersdkauthmethods) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Check initial user created

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/first \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/first`

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create initial user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/first \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/first`

> Body parameter

```json
{
  "email": "string",
  "password": "string",
  "trial": true,
  "username": "string"
}
```

### Parameters

| Name   | In   | Type                                                                         | Required | Description        |
| ------ | ---- | ---------------------------------------------------------------------------- | -------- | ------------------ |
| `body` | body | [codersdk.CreateFirstUserRequest](schemas.md#codersdkcreatefirstuserrequest) | true     | First user request |

### Example responses

> 201 Response

```json
{
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                         |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.CreateFirstUserResponse](schemas.md#codersdkcreatefirstuserresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Log out user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/logout \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/logout`

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## OAuth 2.0 GitHub Callback

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/oauth2/github/callback \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/oauth2/github/callback`

### Responses

| Status | Meaning                                                                 | Description        | Schema |
| ------ | ----------------------------------------------------------------------- | ------------------ | ------ |
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## OpenID Connect Callback

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/oidc/callback \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/oidc/callback`

### Responses

| Status | Meaning                                                                 | Description        | Schema |
| ------ | ----------------------------------------------------------------------- | ------------------ | ------ |
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
