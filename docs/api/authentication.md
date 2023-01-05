# Authentication

> This page is incomplete, stay tuned.

## Log in user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/login \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/login`

> Body parameter

```json
{
  "email": "string",
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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
