# WorkspaceProxies

## Create workspace proxy

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceproxies \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceproxies`

> Body parameter

```json
{
  "display_name": "string",
  "icon": "string",
  "name": "string",
  "url": "string",
  "wildcard_hostname": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description                    |
| ------ | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------------ |
| `body` | body | [codersdk.CreateWorkspaceProxyRequest](schemas.md#codersdkcreateworkspaceproxyrequest) | true     | Create workspace proxy request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "deleted": true,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                       |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.WorkspaceProxy](schemas.md#codersdkworkspaceproxy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete workspace proxy

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/workspaceproxies/{workspaceproxy} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /workspaceproxies/{workspaceproxy}`

### Parameters

| Name             | In   | Type         | Required | Description      |
| ---------------- | ---- | ------------ | -------- | ---------------- |
| `workspaceproxy` | path | string(uuid) | true     | Proxy ID or name |

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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
