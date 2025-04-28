# PortSharing

## Get workspace agent port shares

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/workspaces/{workspace}/port-share \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /workspaces/{workspace}/port-share`

> Body parameter

```json
{
  "agent_name": "string",
  "port": 0
}
```

### Parameters

| Name        | In   | Type                                                                                                     | Required | Description                       |
|-------------|------|----------------------------------------------------------------------------------------------------------|----------|-----------------------------------|
| `workspace` | path | string(uuid)                                                                                             | true     | Workspace ID                      |
| `body`      | body | [codersdk.DeleteWorkspaceAgentPortShareRequest](schemas.md#codersdkdeleteworkspaceagentportsharerequest) | true     | Delete port sharing level request |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert workspace agent port share

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
  "protocol": "http",
  "share_level": "owner"
}
```

### Parameters

| Name        | In   | Type                                                                                                     | Required | Description                       |
|-------------|------|----------------------------------------------------------------------------------------------------------|----------|-----------------------------------|
| `workspace` | path | string(uuid)                                                                                             | true     | Workspace ID                      |
| `body`      | body | [codersdk.UpsertWorkspaceAgentPortShareRequest](schemas.md#codersdkupsertworkspaceagentportsharerequest) | true     | Upsert port sharing level request |

### Example responses

> 200 Response

```json
{
  "agent_name": "string",
  "port": 0,
  "protocol": "http",
  "share_level": "owner",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                         |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentPortShare](schemas.md#codersdkworkspaceagentportshare) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
