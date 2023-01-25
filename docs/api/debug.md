# Debug

## Debug Info Wireguard Coordinator

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/debug/coordinator \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /debug/coordinator`

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
