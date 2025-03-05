# Users

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
|------------|-------|--------------|----------|--------------|
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
      "login_type": "",
      "name": "string",
      "organization_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "roles": [
        {
          "display_name": "string",
          "name": "string",
          "organization_id": "string"
        }
      ],
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
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
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "password": "string",
  "user_status": "active",
  "username": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description         |
|--------|------|------------------------------------------------------------------------------------|----------|---------------------|
| `body` | body | [codersdk.CreateUserRequestWithOrgs](schemas.md#codersdkcreateuserrequestwithorgs) | true     | Create user request |

### Example responses

> 201 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
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
  "github": {
    "default_provider_configured": true,
    "enabled": true
  },
  "oidc": {
    "enabled": true,
    "iconUrl": "string",
    "signInText": "string"
  },
  "password": {
    "enabled": true
  },
  "terms_of_service_url": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                 |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------|
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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
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
  "name": "string",
  "password": "string",
  "trial": true,
  "trial_info": {
    "company_name": "string",
    "country": "string",
    "developers": "string",
    "first_name": "string",
    "job_title": "string",
    "last_name": "string",
    "phone_number": "string"
  },
  "username": "string"
}
```

### Parameters

| Name   | In   | Type                                                                         | Required | Description        |
|--------|------|------------------------------------------------------------------------------|----------|--------------------|
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
|--------|--------------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
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
|--------|-------------------------------------------------------------------------|--------------------|--------|
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get Github device auth

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/oauth2/github/device \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/oauth2/github/device`

### Example responses

> 200 Response

```json
{
  "device_code": "string",
  "expires_in": 0,
  "interval": 0,
  "user_code": "string",
  "verification_uri": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ExternalAuthDevice](schemas.md#codersdkexternalauthdevice) |

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
|--------|-------------------------------------------------------------------------|--------------------|--------|
| 307    | [Temporary Redirect](https://tools.ietf.org/html/rfc7231#section-6.4.7) | Temporary Redirect |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user by name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}`

### Parameters

| Name   | In   | Type   | Required | Description              |
|--------|------|--------|----------|--------------------------|
| `user` | path | string | true     | User ID, username, or me |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete user

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/users/{user} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /users/{user}`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user appearance settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/appearance \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/appearance`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "theme_preference": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserAppearanceSettings](schemas.md#codersdkuserappearancesettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user appearance settings

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/appearance \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/appearance`

> Body parameter

```json
{
  "theme_preference": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                   | Required | Description             |
|--------|------|--------------------------------------------------------------------------------------------------------|----------|-------------------------|
| `user` | path | string                                                                                                 | true     | User ID, name, or me    |
| `body` | body | [codersdk.UpdateUserAppearanceSettingsRequest](schemas.md#codersdkupdateuserappearancesettingsrequest) | true     | New appearance settings |

### Example responses

> 200 Response

```json
{
  "theme_preference": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserAppearanceSettings](schemas.md#codersdkuserappearancesettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get autofill build parameters for user

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/autofill-parameters?template_id=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/autofill-parameters`

### Parameters

| Name          | In    | Type   | Required | Description              |
|---------------|-------|--------|----------|--------------------------|
| `user`        | path  | string | true     | User ID, username, or me |
| `template_id` | query | string | true     | Template ID              |

### Example responses

> 200 Response

```json
[
  {
    "name": "string",
    "value": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                              |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserParameter](schemas.md#codersdkuserparameter) |

<h3 id="get-autofill-build-parameters-for-user-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `[array item]` | array  | false    |              |             |
| `» name`       | string | false    |              |             |
| `» value`      | string | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user Git SSH key

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/gitsshkey \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/gitsshkey`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "public_key": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GitSSHKey](schemas.md#codersdkgitsshkey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Regenerate user SSH key

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/gitsshkey \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/gitsshkey`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "public_key": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GitSSHKey](schemas.md#codersdkgitsshkey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create new session key

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/keys \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/keys`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 201 Response

```json
{
  "key": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                       |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.GenerateAPIKeyResponse](schemas.md#codersdkgenerateapikeyresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user tokens

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/keys/tokens \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/keys/tokens`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "expires_at": "2019-08-24T14:15:22Z",
    "id": "string",
    "last_used": "2019-08-24T14:15:22Z",
    "lifetime_seconds": 0,
    "login_type": "password",
    "scope": "all",
    "token_name": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.APIKey](schemas.md#codersdkapikey) |

<h3 id="get-user-tokens-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                   | Required | Restrictions | Description |
|----------------------|--------------------------------------------------------|----------|--------------|-------------|
| `[array item]`       | array                                                  | false    |              |             |
| `» created_at`       | string(date-time)                                      | true     |              |             |
| `» expires_at`       | string(date-time)                                      | true     |              |             |
| `» id`               | string                                                 | true     |              |             |
| `» last_used`        | string(date-time)                                      | true     |              |             |
| `» lifetime_seconds` | integer                                                | true     |              |             |
| `» login_type`       | [codersdk.LoginType](schemas.md#codersdklogintype)     | true     |              |             |
| `» scope`            | [codersdk.APIKeyScope](schemas.md#codersdkapikeyscope) | true     |              |             |
| `» token_name`       | string                                                 | true     |              |             |
| `» updated_at`       | string(date-time)                                      | true     |              |             |
| `» user_id`          | string(uuid)                                           | true     |              |             |

#### Enumerated Values

| Property     | Value                 |
|--------------|-----------------------|
| `login_type` | `password`            |
| `login_type` | `github`              |
| `login_type` | `oidc`                |
| `login_type` | `token`               |
| `scope`      | `all`                 |
| `scope`      | `application_connect` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create token API key

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/keys/tokens \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/keys/tokens`

> Body parameter

```json
{
  "lifetime": 0,
  "scope": "all",
  "token_name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                 | Required | Description          |
|--------|------|----------------------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                               | true     | User ID, name, or me |
| `body` | body | [codersdk.CreateTokenRequest](schemas.md#codersdkcreatetokenrequest) | true     | Create token request |

### Example responses

> 201 Response

```json
{
  "key": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                       |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.GenerateAPIKeyResponse](schemas.md#codersdkgenerateapikeyresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get API key by token name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/keys/tokens/{keyname} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/keys/tokens/{keyname}`

### Parameters

| Name      | In   | Type           | Required | Description          |
|-----------|------|----------------|----------|----------------------|
| `user`    | path | string         | true     | User ID, name, or me |
| `keyname` | path | string(string) | true     | Key Name             |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "expires_at": "2019-08-24T14:15:22Z",
  "id": "string",
  "last_used": "2019-08-24T14:15:22Z",
  "lifetime_seconds": 0,
  "login_type": "password",
  "scope": "all",
  "token_name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.APIKey](schemas.md#codersdkapikey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get API key by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/keys/{keyid} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/keys/{keyid}`

### Parameters

| Name    | In   | Type         | Required | Description          |
|---------|------|--------------|----------|----------------------|
| `user`  | path | string       | true     | User ID, name, or me |
| `keyid` | path | string(uuid) | true     | Key ID               |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "expires_at": "2019-08-24T14:15:22Z",
  "id": "string",
  "last_used": "2019-08-24T14:15:22Z",
  "lifetime_seconds": 0,
  "login_type": "password",
  "scope": "all",
  "token_name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.APIKey](schemas.md#codersdkapikey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete API key

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/users/{user}/keys/{keyid} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /users/{user}/keys/{keyid}`

### Parameters

| Name    | In   | Type         | Required | Description          |
|---------|------|--------------|----------|----------------------|
| `user`  | path | string       | true     | User ID, name, or me |
| `keyid` | path | string(uuid) | true     | Key ID               |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user login type

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/login-type \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/login-type`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "login_type": ""
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                     |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserLoginType](schemas.md#codersdkuserlogintype) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get organizations by user

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/organizations \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/organizations`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "description": "string",
    "display_name": "string",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "is_default": true,
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Organization](schemas.md#codersdkorganization) |

<h3 id="get-organizations-by-user-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description |
|------------------|-------------------|----------|--------------|-------------|
| `[array item]`   | array             | false    |              |             |
| `» created_at`   | string(date-time) | true     |              |             |
| `» description`  | string            | false    |              |             |
| `» display_name` | string            | false    |              |             |
| `» icon`         | string            | false    |              |             |
| `» id`           | string(uuid)      | true     |              |             |
| `» is_default`   | boolean           | true     |              |             |
| `» name`         | string            | false    |              |             |
| `» updated_at`   | string(date-time) | true     |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get organization by user and organization name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/organizations/{organizationname} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/organizations/{organizationname}`

### Parameters

| Name               | In   | Type   | Required | Description          |
|--------------------|------|--------|----------|----------------------|
| `user`             | path | string | true     | User ID, name, or me |
| `organizationname` | path | string | true     | Organization name    |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user password

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/password \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/password`

> Body parameter

```json
{
  "old_password": "string",
  "password": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description             |
|--------|------|------------------------------------------------------------------------------------|----------|-------------------------|
| `user` | path | string                                                                             | true     | User ID, name, or me    |
| `body` | body | [codersdk.UpdateUserPasswordRequest](schemas.md#codersdkupdateuserpasswordrequest) | true     | Update password request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user profile

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/profile \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/profile`

> Body parameter

```json
{
  "name": "string",
  "username": "string"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description          |
|--------|------|----------------------------------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                                           | true     | User ID, name, or me |
| `body` | body | [codersdk.UpdateUserProfileRequest](schemas.md#codersdkupdateuserprofilerequest) | true     | Updated profile      |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user roles

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/roles`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Assign role to user

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/roles`

> Body parameter

```json
{
  "roles": [
    "string"
  ]
}
```

### Parameters

| Name   | In   | Type                                                   | Required | Description          |
|--------|------|--------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                 | true     | User ID, name, or me |
| `body` | body | [codersdk.UpdateRoles](schemas.md#codersdkupdateroles) | true     | Update roles request |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Activate user account

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/status/activate \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/status/activate`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Suspend user account

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/status/suspend \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/status/suspend`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
