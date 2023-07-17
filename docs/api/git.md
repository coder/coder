# Git

## Get git auth by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/gitauth/{gitauth} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /gitauth/{gitauth}`

### Parameters

| Name      | In   | Type           | Required | Description     |
| --------- | ---- | -------------- | -------- | --------------- |
| `gitauth` | path | string(string) | true     | Git Provider ID |

### Example responses

> 200 Response

```json
{
  "app_install_url": "string",
  "app_installable": true,
  "authenticated": true,
  "device": true,
  "installations": [
    {
      "account": {
        "avatar_url": "string",
        "login": "string",
        "name": "string",
        "profile_url": "string"
      },
      "configure_url": "string",
      "id": 0
    }
  ],
  "type": "string",
  "user": {
    "avatar_url": "string",
    "login": "string",
    "name": "string",
    "profile_url": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                         |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GitAuth](schemas.md#codersdkgitauth) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get git auth device by ID.

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/gitauth/{gitauth}/device \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /gitauth/{gitauth}/device`

### Parameters

| Name      | In   | Type           | Required | Description     |
| --------- | ---- | -------------- | -------- | --------------- |
| `gitauth` | path | string(string) | true     | Git Provider ID |

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

| Status | Meaning                                                 | Description | Schema                                                     |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GitAuthDevice](schemas.md#codersdkgitauthdevice) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Post git auth device by ID

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/gitauth/{gitauth}/device \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /gitauth/{gitauth}/device`

### Parameters

| Name      | In   | Type           | Required | Description     |
| --------- | ---- | -------------- | -------- | --------------- |
| `gitauth` | path | string(string) | true     | Git Provider ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
