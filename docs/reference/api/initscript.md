# InitScript

## Get agent init script

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/init-script/{os}/{arch}

```

`GET /init-script/{os}/{arch}`

### Parameters

| Name   | In   | Type   | Required | Description      |
|--------|------|--------|----------|------------------|
| `os`   | path | string | true     | Operating system |
| `arch` | path | string | true     | Architecture     |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | Success     |        |
