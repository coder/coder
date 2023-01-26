# Insights

## Get deployment DAUs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/insights/daus \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /insights/daus`

### Example responses

> 200 Response

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DeploymentDAUsResponse](schemas.md#codersdkdeploymentdausresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
