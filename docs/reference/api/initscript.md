# InitScript

## Get agent init script

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/init-script

```

`GET /init-script`

### Parameters

| Name   | In    | Type   | Required | Description      |
|--------|-------|--------|----------|------------------|
| `os`   | query | string | false    | Operating system |
| `arch` | query | string | false    | Architecture     |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | Success     |        |
