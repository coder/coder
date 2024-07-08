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
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "*"
      }
    },
    "property2": {
      "action": "create",
      "object": {
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
| ------ | ---- | ------------------------------------------------------------------------ | -------- | --------------------- |
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
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------- |
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
| ------ | ---- | -------------------------------------------------------------------------------- | -------- | ------------- |
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
| ------ | ------------------------------------------------------------ | ----------- | ---------------------------------------------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.LoginWithPasswordResponse](schemas.md#codersdkloginwithpasswordresponse) |

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
| ------ | ---- | ---------------------------------------------------------------------- | -------- | -------------------- |
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
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.OAuthConversionResponse](schemas.md#codersdkoauthconversionresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
