# PortSharing

## Create workspace agent port share

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaces/{workspace}/port-share \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaces/{workspace}/port-share`

> Body parameter

```json
{
  "agent_name": "string",
  "port": 0,
  "share_level": "owner"
}
```

### Parameters

| Name        | In   | Type                                                                                                     | Required | Description                       |
| ----------- | ---- | -------------------------------------------------------------------------------------------------------- | -------- | --------------------------------- |
| `workspace` | path | string(uuid)                                                                                             | true     | Workspace ID                      |
| `body`      | body | [codersdk.UpdateWorkspaceAgentPortShareRequest](schemas.md#codersdkupdateworkspaceagentportsharerequest) | true     | Create port sharing level request |

### Example responses

> 200 Response

```json
{
  "agent_name": "string",
  "port": 0,
  "share_level": "owner"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                         |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentPortShare](schemas.md#codersdkworkspaceagentportshare) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace agent port shares

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaces/{workspace}/port-shares \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaces/{workspace}/port-shares`

### Parameters

| Name        | In   | Type         | Required | Description  |
| ----------- | ---- | ------------ | -------- | ------------ |
| `workspace` | path | string(uuid) | true     | Workspace ID |

### Example responses

> 200 Response

```json
{
  "shares": [
    {
      "agent_name": "string",
      "port": 0,
      "share_level": "owner"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentPortShares](schemas.md#codersdkworkspaceagentportshares) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
