# Default

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
| -------------- | ---- | -------------- | -------- | --------------- |
| `externalauth` | path | string(string) | true     | Git Provider ID |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
