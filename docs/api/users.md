# Users

> This page is incomplete, stay tuned.

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
