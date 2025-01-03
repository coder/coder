# Git

## Get user external auths

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/external-auth \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /external-auth`

### Example responses

> 200 Response

```json
{
  "authenticated": true,
  "created_at": "2019-08-24T14:15:22Z",
  "expires": "2019-08-24T14:15:22Z",
  "has_refresh_token": true,
  "provider_id": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "validate_error": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ExternalAuthLink](schemas.md#codersdkexternalauthlink) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get external auth by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/external-auth/{externalauth} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /external-auth/{externalauth}`

### Parameters

| Name           | In   | Type           | Required | Description     |
|----------------|------|----------------|----------|-----------------|
| `externalauth` | path | string(string) | true     | Git Provider ID |

### Example responses

> 200 Response

```json
{
  "app_install_url": "string",
  "app_installable": true,
  "authenticated": true,
  "device": true,
  "display_name": "string",
  "installations": [
    {
      "account": {
        "avatar_url": "string",
        "id": 0,
        "login": "string",
        "name": "string",
        "profile_url": "string"
      },
      "configure_url": "string",
      "id": 0
    }
  ],
  "user": {
    "avatar_url": "string",
    "id": 0,
    "login": "string",
    "name": "string",
    "profile_url": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ExternalAuth](schemas.md#codersdkexternalauth) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete external auth user link by ID

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/external-auth/{externalauth} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /external-auth/{externalauth}`

### Parameters

| Name           | In   | Type           | Required | Description     |
|----------------|------|----------------|----------|-----------------|
| `externalauth` | path | string(string) | true     | Git Provider ID |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get external auth device by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/external-auth/{externalauth}/device \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /external-auth/{externalauth}/device`

### Parameters

| Name           | In   | Type           | Required | Description     |
|----------------|------|----------------|----------|-----------------|
| `externalauth` | path | string(string) | true     | Git Provider ID |

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

## Post external auth device by ID

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/external-auth/{externalauth}/device \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /external-auth/{externalauth}/device`

### Parameters

| Name           | In   | Type           | Required | Description          |
|----------------|------|----------------|----------|----------------------|
| `externalauth` | path | string(string) | true     | External Provider ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
