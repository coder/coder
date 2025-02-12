# Authorization

## Check authorization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/authcheck \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /authcheck`

> Body parameter

```json
{
  "checks": {
    "property1": {
      "action": "create",
      "object": {
        "any_org": true,
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "*"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "any_org": true,
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "*"
      }
    }
  }
}
```

### Parameters

| Name   | In   | Type                                                                     | Required | Description           |
|--------|------|--------------------------------------------------------------------------|----------|-----------------------|
| `body` | body | [codersdk.AuthorizationRequest](schemas.md#codersdkauthorizationrequest) | true     | Authorization request |

### Example responses

> 200 Response

```json
{
  "property1": true,
  "property2": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AuthorizationResponse](schemas.md#codersdkauthorizationresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Log in user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/login \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json'
```

`POST /users/login`

> Body parameter

```json
{
  "email": "user@example.com",
  "password": "string"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description   |
|--------|------|----------------------------------------------------------------------------------|----------|---------------|
| `body` | body | [codersdk.LoginWithPasswordRequest](schemas.md#codersdkloginwithpasswordrequest) | true     | Login request |

### Example responses

> 201 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                             |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.LoginWithPasswordResponse](schemas.md#codersdkloginwithpasswordresponse) |

## Change password with a one-time passcode

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/otp/change-password \
  -H 'Content-Type: application/json'
```

`POST /users/otp/change-password`

> Body parameter

```json
{
  "email": "user@example.com",
  "one_time_passcode": "string",
  "password": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                             | Required | Description             |
|--------|------|------------------------------------------------------------------------------------------------------------------|----------|-------------------------|
| `body` | body | [codersdk.ChangePasswordWithOneTimePasscodeRequest](schemas.md#codersdkchangepasswordwithonetimepasscoderequest) | true     | Change password request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

## Request one-time passcode

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/otp/request \
  -H 'Content-Type: application/json'
```

`POST /users/otp/request`

> Body parameter

```json
{
  "email": "user@example.com"
}
```

### Parameters

| Name   | In   | Type                                                                                       | Required | Description               |
|--------|------|--------------------------------------------------------------------------------------------|----------|---------------------------|
| `body` | body | [codersdk.RequestOneTimePasscodeRequest](schemas.md#codersdkrequestonetimepasscoderequest) | true     | One-time passcode request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

## Validate user password

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/validate-password \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/validate-password`

> Body parameter

```json
{
  "password": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description                    |
|--------|------|----------------------------------------------------------------------------------------|----------|--------------------------------|
| `body` | body | [codersdk.ValidateUserPasswordRequest](schemas.md#codersdkvalidateuserpasswordrequest) | true     | Validate user password request |

### Example responses

> 200 Response

```json
{
  "details": "string",
  "valid": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ValidateUserPasswordResponse](schemas.md#codersdkvalidateuserpasswordresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Convert user from password to oauth authentication

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/convert-login \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/convert-login`

> Body parameter

```json
{
  "password": "string",
  "to_type": ""
}
```

### Parameters

| Name   | In   | Type                                                                   | Required | Description          |
|--------|------|------------------------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                                 | true     | User ID, name, or me |
| `body` | body | [codersdk.ConvertLoginRequest](schemas.md#codersdkconvertloginrequest) | true     | Convert request      |

### Example responses

> 201 Response

```json
{
  "expires_at": "2019-08-24T14:15:22Z",
  "state_string": "string",
  "to_type": "",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                         |
|--------|--------------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.OAuthConversionResponse](schemas.md#codersdkoauthconversionresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
